package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// AddRemediation appends to an incident's audit log and publishes
// `remediation.added` on the event bus. The stored remediation is
// canonicalised: ID gets a fresh ULID (ignoring client-supplied ID —
// clients can't pre-mint IDs), IncidentID is taken from the URL path
// (ignoring body), ExecutedAt is stamped now if absent.
func (s *Server) AddRemediation(w http.ResponseWriter, r *http.Request, incidentID string) {
	if _, err := s.storage.GetIncident(r.Context(), incidentID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND",
				"incident "+incidentID+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	var body gen.Remediation
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}

	now := time.Now().UTC()
	executedAt := body.ExecutedAt
	if executedAt.IsZero() {
		executedAt = now
	}

	rem := domain.Remediation{
		ID:         ulid.Make().String(),
		IncidentID: incidentID,
		Kind:       domain.RemediationKind(body.Kind),
		ExecutedAt: executedAt,
	}
	if body.ActorRef != nil {
		rem.ActorRef = *body.ActorRef
	}
	if body.Details != nil {
		rem.Details = *body.Details
	}
	if err := rem.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REMEDIATION_KIND", err.Error())
		return
	}

	if err := s.storage.AppendRemediation(r.Context(), incidentID, rem); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	s.events.Publish(domain.RemediationAddedEvent{
		IncidentID:    incidentID,
		RemediationID: rem.ID,
		Kind:          rem.Kind,
		At:            now,
	})
	writeJSON(w, http.StatusCreated, toGenRemediation(rem))
}
