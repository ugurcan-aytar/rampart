// Package api is the HTTP entrypoint for the engine. Routes are served by
// the OpenAPI-generated mux (gen.Handler); this file implements
// gen.ServerInterface on top of the storage + trust stack.
//
// Adım 3 status: Healthz + Readyz + ListComponents are live; every other
// method returns 501 with an explanatory Error body. The SSE endpoint
// (/v1/stream) returns 501 until the SSE adapter ADR is approved —
// see engine/internal/api/sse.go (not yet written).
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Server implements gen.ServerInterface over a storage backend and an
// in-process event bus. The bus + heartbeat are injected rather than
// constructed here so tests can drive different timing / buffer sizes.
type Server struct {
	storage           storage.Storage
	events            *events.Bus
	heartbeatInterval time.Duration
	log               *slog.Logger
}

// NewServer wires a Server against the storage + event bus + heartbeat.
// Production callers pass config.Default()-derived values; tests override.
func NewServer(s storage.Storage, bus *events.Bus, heartbeat time.Duration) *Server {
	return &Server{
		storage:           s,
		events:            bus,
		heartbeatInterval: heartbeat,
		log:               slog.Default(),
	}
}

// Handler returns the OpenAPI-generated mux with the Server's methods bound.
func (s *Server) Handler() http.Handler {
	return gen.Handler(s)
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

// --- 501 stubs -------------------------------------------------------------

func (s *Server) BlastRadius(w http.ResponseWriter, _ *http.Request) {
	writeNotImplemented(w, "BlastRadius")
}

func (s *Server) UpsertComponent(w http.ResponseWriter, _ *http.Request) {
	writeNotImplemented(w, "UpsertComponent")
}

func (s *Server) ListSBOMsByComponent(w http.ResponseWriter, _ *http.Request, _ gen.ComponentRef) {
	writeNotImplemented(w, "ListSBOMsByComponent")
}

func (s *Server) SubmitSBOM(w http.ResponseWriter, _ *http.Request, _ gen.ComponentRef) {
	writeNotImplemented(w, "SubmitSBOM")
}

func (s *Server) ListIncidents(w http.ResponseWriter, _ *http.Request, _ gen.ListIncidentsParams) {
	writeNotImplemented(w, "ListIncidents")
}

func (s *Server) GetIncident(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "GetIncident")
}

func (s *Server) AddRemediation(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "AddRemediation")
}

func (s *Server) TransitionIncident(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "TransitionIncident")
}

func (s *Server) ListIoCs(w http.ResponseWriter, _ *http.Request, _ gen.ListIoCsParams) {
	writeNotImplemented(w, "ListIoCs")
}

func (s *Server) SubmitIoC(w http.ResponseWriter, _ *http.Request) {
	writeNotImplemented(w, "SubmitIoC")
}

func (s *Server) GetSBOM(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "GetSBOM")
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

func writeNotImplemented(w http.ResponseWriter, op string) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		op+" is not implemented yet — see ROADMAP.md Phase 1.")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, gen.Error{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
