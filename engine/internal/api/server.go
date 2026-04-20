// Package api is the HTTP entrypoint for the engine. Phase 1 ships healthz
// and readyz; the OpenAPI-driven handlers land in Adım 3.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

type Server struct {
	storage storage.Storage
	log     *slog.Logger
}

func NewServer(s storage.Storage) *Server {
	return &Server{storage: s, log: slog.Default()}
}

// Handler returns the full HTTP handler, ready to be installed on an
// http.Server. Adım 3 extends this with the /v1/* endpoints defined in
// schemas/openapi.yaml.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	// Real readiness (storage ping, trust engine probe) lands in Adım 3.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
