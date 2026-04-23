// Package api is the HTTP entrypoint for the engine. Routes are served by
// the OpenAPI-generated mux (gen.Handler); this file implements
// gen.ServerInterface on top of the storage + trust stack.
//
// Handler surface at Adım 7 close:
//   - Healthz / Readyz — live
//   - /v1/components — GET (list), POST (upsert)
//   - /v1/components/{ref}/sboms — GET (list), POST (submit + match)
//   - /v1/sboms/{id} — GET
//   - /v1/iocs — GET (list), POST (submit + forward match)
//   - /v1/incidents — GET (list), /v1/incidents/{id} — GET
//   - /v1/incidents/{id}/transition — POST
//   - /v1/incidents/{id}/remediations — POST
//   - /v1/blast-radius — POST
//   - /v1/stream — GET (SSE, Adım 3)
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// SBOMParser is the subset of parser behaviour the HTTP layer needs.
// `engine/sbom/npm.Parser` and `engine/sbom/npm.StrategyParser` both
// satisfy it; swapping the Go parser for the Rust-sidecar strategy is
// a call to [Server.SetParser] and nothing else.
type SBOMParser interface {
	Parse(ctx context.Context, content []byte) (*domain.ParsedSBOM, error)
}

// Server implements gen.ServerInterface over a storage backend and an
// in-process event bus. The bus + heartbeat are injected rather than
// constructed here so tests can drive different timing / buffer sizes.
type Server struct {
	storage           storage.Storage
	events            *events.Bus
	heartbeatInterval time.Duration
	log               *slog.Logger
	parser            SBOMParser

	auth middleware.AuthOptions
}

// NewServer wires a Server against the storage + event bus + heartbeat.
// Production callers pass config.Default()-derived values; tests override.
// The parser defaults to the embedded Go parser — `SetParser` swaps in
// the native strategy for deployments that opt in via `--profile native`.
func NewServer(s storage.Storage, bus *events.Bus, heartbeat time.Duration) *Server {
	return &Server{
		storage:           s,
		events:            bus,
		heartbeatInterval: heartbeat,
		log:               slog.Default(),
		parser:            npm.NewParser(),
	}
}

// SetParser replaces the SBOM parser backend. Intended to be called once
// at app boot after `npm.EffectiveStrategy` resolves the runtime choice.
func (s *Server) SetParser(p SBOMParser) { s.parser = p }

// SetAuth installs the JWT validation options. Defaults to disabled,
// i.e. the v0.1.x passthrough behaviour; production deployments set
// RAMPART_AUTH_ENABLED=true + RAMPART_AUTH_SIGNING_KEY to engage it.
func (s *Server) SetAuth(opts middleware.AuthOptions) {
	if len(opts.ExemptPaths) == 0 {
		opts.ExemptPaths = middleware.DefaultExemptPaths
	}
	s.auth = opts
}

// Handler returns the OpenAPI-generated mux wrapped in CORS + auth
// middleware. Order matters: CORS is outermost so preflight OPTIONS
// requests never reach the auth layer; auth runs next so SSE / health
// / `/v1/auth/token` get exempted by prefix before any handler runs.
func (s *Server) Handler() http.Handler {
	h := gen.Handler(s)
	h = middleware.Auth(s.auth)(h)
	h = corsMiddleware(h)
	return h
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Compile-time interface check — any schema change that adds an operation
// will break this assertion until the new method is implemented.
var _ gen.ServerInterface = (*Server)(nil)

// --- Live handlers ---------------------------------------------------------

func (s *Server) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, gen.Health{Status: gen.Ok})
}

func (s *Server) Readyz(w http.ResponseWriter, _ *http.Request) {
	// Real readiness (storage probe, trust engine liveness) lands alongside
	// the incident logic; for now, always-ready matches Adım 2 behaviour.
	writeJSON(w, http.StatusOK, gen.Health{Status: gen.Ready})
}

func (s *Server) ListComponents(w http.ResponseWriter, r *http.Request, _ gen.ListComponentsParams) {
	comps, err := s.storage.ListComponents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	items := make([]gen.Component, 0, len(comps))
	for _, c := range comps {
		items = append(items, toGenComponent(c))
	}
	writeJSON(w, http.StatusOK, gen.ComponentPage{Items: items})
}

// Stream is the SSE endpoint. The adapter lives in sse.go; this handler
// just sets up the framer, subscribes to the event bus, and enters the
// loop. Disconnect cleanup is handled by streamLoop's ctx-done branch.
func (s *Server) Stream(w http.ResponseWriter, r *http.Request, params gen.StreamParams) {
	sse, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", err.Error())
		return
	}

	// Phase 1 is hot-only: we log the Last-Event-ID so we can grep the
	// signal to size Phase 2's replay buffer when it lands.
	if params.LastEventID != nil && *params.LastEventID != "" {
		s.log.Info("sse: client requested replay but only hot-delivery is supported in Phase 1",
			"last_event_id", *params.LastEventID)
	}

	ch, cancel := s.events.Subscribe(r.Context())
	defer cancel()

	streamLoop(r.Context(), s.log, sse, ch, s.heartbeatInterval)
}

// --- helpers ---------------------------------------------------------------

func toGenComponent(c domain.Component) gen.Component {
	g := gen.Component{
		Ref:       c.Ref,
		Kind:      c.Kind,
		Namespace: c.Namespace,
		Name:      c.Name,
	}
	if c.Owner != "" {
		owner := c.Owner
		g.Owner = &owner
	}
	if c.System != "" {
		sys := c.System
		g.System = &sys
	}
	if c.Lifecycle != "" {
		lc := gen.ComponentLifecycle(c.Lifecycle)
		g.Lifecycle = &lc
	}
	if len(c.Tags) > 0 {
		tags := c.Tags
		g.Tags = &tags
	}
	if len(c.Annotations) > 0 {
		ann := c.Annotations
		g.Annotations = &ann
	}
	return g
}

func fromGenComponent(g gen.Component) domain.Component {
	c := domain.Component{
		Ref:       g.Ref,
		Kind:      g.Kind,
		Namespace: g.Namespace,
		Name:      g.Name,
	}
	if g.Owner != nil {
		c.Owner = *g.Owner
	}
	if g.System != nil {
		c.System = *g.System
	}
	if g.Lifecycle != nil {
		c.Lifecycle = string(*g.Lifecycle)
	}
	if g.Tags != nil {
		c.Tags = *g.Tags
	}
	if g.Annotations != nil {
		c.Annotations = *g.Annotations
	}
	return c
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, gen.Error{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
