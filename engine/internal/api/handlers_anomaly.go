package api

import (
	"errors"
	"net/http"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// defaultAnomalyLimit caps a single ListAnomalies response when the
// caller does not supply ?limit=. Mirrors the OpenAPI default of 100.
const defaultAnomalyLimit = 100

// ListAnomalies implements `GET /v1/anomalies`. Returns newest-first.
// All filter dimensions are optional; an empty filter returns the full
// table (capped at limit).
func (s *Server) ListAnomalies(w http.ResponseWriter, r *http.Request, params gen.ListAnomaliesParams) {
	filter := domain.AnomalyFilter{Limit: defaultAnomalyLimit}
	if params.PackageRef != nil {
		filter.PackageRef = *params.PackageRef
	}
	if params.Kind != nil {
		filter.Kind = domain.AnomalyKind(*params.Kind)
	}
	if params.From != nil {
		t := *params.From
		filter.From = &t
	}
	if params.To != nil {
		t := *params.To
		filter.To = &t
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}

	rows, err := s.storage.ListAnomalies(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	out := gen.AnomalyPage{Items: make([]gen.Anomaly, 0, len(rows))}
	for _, a := range rows {
		out.Items = append(out.Items, anomalyToWire(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetAnomaly implements `GET /v1/anomalies/{id}`.
func (s *Server) GetAnomaly(w http.ResponseWriter, r *http.Request, id int64) {
	a, err := s.storage.GetAnomaly(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ANOMALY_NOT_FOUND",
				"no anomaly with that id")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, anomalyToWire(*a))
}

// anomalyToWire mirrors the domain Anomaly onto the generated OpenAPI
// shape. Evidence map is sent as-is; nil maps are omitted.
func anomalyToWire(a domain.Anomaly) gen.Anomaly {
	out := gen.Anomaly{
		Id:          a.ID,
		Kind:        gen.AnomalyKind(a.Kind),
		PackageRef:  a.PackageRef,
		DetectedAt:  a.DetectedAt,
		Confidence:  gen.Confidence(a.Confidence),
		Explanation: a.Explanation,
	}
	if len(a.Evidence) > 0 {
		ev := map[string]any(a.Evidence)
		out.Evidence = &ev
	}
	return out
}
