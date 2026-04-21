package api

import (
	"errors"
	"net/http"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// UpsertComponent registers or updates a component. Idempotent: a second
// POST with the same `ref` overwrites the stored component. Response
// carries the stored record — important when the server fills in derived
// fields (kind/namespace/name parsed from ref) the client didn't bother
// computing.
func (s *Server) UpsertComponent(w http.ResponseWriter, r *http.Request) {
	var body gen.Component
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}
	if body.Ref == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "ref is required")
		return
	}
	// Parse the ref to validate shape; body's kind/namespace/name are
	// ignored in favour of the ref-derived values — single source of
	// truth (domain.ParseComponentRef) prevents the two drifting.
	kind, ns, name, err := domain.ParseComponentRef(body.Ref)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REF", err.Error())
		return
	}
	c := fromGenComponent(body)
	c.Kind = kind
	c.Namespace = ns
	c.Name = name

	// Distinguish 201 (new) from 200 (update) so operators' pipelines can
	// act on the difference — e.g., scaffolder only enqueues retroactive
	// match on first registration.
	status := http.StatusCreated
	if _, err := s.storage.GetComponent(r.Context(), c.Ref); err == nil {
		status = http.StatusOK
	} else if !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	if err := s.storage.UpsertComponent(r.Context(), c); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	writeJSON(w, status, toGenComponent(c))
}
