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

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

type Server struct {
	storage storage.Storage
	log     *slog.Logger
}

func NewServer(s storage.Storage) *Server {
	return &Server{storage: s, log: slog.Default()}
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

// Stream is the SSE endpoint; the real adapter lives in sse.go once the ADR
// is approved. The stub returns 501 so gen.ServerInterface is satisfied.
func (s *Server) Stream(w http.ResponseWriter, _ *http.Request, _ gen.StreamParams) {
	writeNotImplemented(w, "Stream (SSE adapter pending ADR)")
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
