package api

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/matcher"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// ListIncidents returns incidents filtered by state / ecosystem / since.
// Sorted by OpenedAt desc (newest first — the Backstage IncidentDashboard
// reads the list top-down and operators look at the most recent first).
// Cursor pagination is Phase 2; the params are accepted and ignored so
// clients that already send them don't break.
func (s *Server) ListIncidents(w http.ResponseWriter, r *http.Request, params gen.ListIncidentsParams) {
	incs, err := s.storage.ListIncidents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	filtered := incs[:0]
	for _, inc := range incs {
		if params.State != nil && domain.IncidentState(*params.State) != inc.State {
			continue
		}
		if params.Since != nil && inc.OpenedAt.Before(*params.Since) {
			continue
		}
		if params.Ecosystem != nil && *params.Ecosystem != "" {
			// Ecosystem filter routes through the linked IoC; join on the fly.
			// A listing endpoint doing a join every call would be a problem in
			// production, but Phase 1 memory storage with <100 incidents is
			// fine. Phase 2 storage builds the right index.
			ioc, err := s.storage.GetIoC(r.Context(), inc.IoCID)
			if err != nil || ioc == nil || ioc.Ecosystem != *params.Ecosystem {
				continue
			}
		}
		filtered = append(filtered, inc)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].OpenedAt.After(filtered[j].OpenedAt)
	})

	items := make([]gen.Incident, 0, len(filtered))
	for _, inc := range filtered {
		items = append(items, toGenIncident(inc))
	}
	writeJSON(w, http.StatusOK, gen.IncidentPage{Items: items})
}

// GetIncident returns a single incident. Snapshot + remediations come
// back as-is; the linked IoC is not hydrated today — clients who need
// IoC detail hit /v1/iocs?… themselves. That's a deliberate Phase 1
// trade-off (keeps the handler stateless; no N+1 joins in memory
// storage). Phase 2 denormalises if the UI proves the join is a hot
// path.
func (s *Server) GetIncident(w http.ResponseWriter, r *http.Request, id string) {
	inc, err := s.storage.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident "+id+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toGenIncident(*inc))
}

// TransitionIncident advances an incident's state machine. Calls through
// to domain.Incident.Transition; invalid transitions return 409 with
// the domain's error message so operators see "pending → closed is not
// allowed", not just "409". Idempotent self-transitions succeed with
// no event emission.
func (s *Server) TransitionIncident(w http.ResponseWriter, r *http.Request, id string) {
	var body gen.IncidentTransitionRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}

	inc, err := s.storage.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident "+id+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	from := inc.State
	target := domain.IncidentState(body.To)
	now := time.Now().UTC()
	if err := inc.Transition(target, now); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_STATE", err.Error())
		return
	}

	if err := s.storage.UpsertIncident(r.Context(), *inc); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	// No event on a self-transition — the Incident.Transition did no work.
	if from != inc.State {
		s.events.Publish(domain.IncidentTransitionedEvent{
			IncidentID: inc.ID,
			From:       from,
			To:         inc.State,
			At:         now,
		})
	}
	writeJSON(w, http.StatusOK, toGenIncident(*inc))
}

// BlastRadius answers "given these IoCs, which components are affected?"
// without opening incidents. Useful for what-if analysis: "a new IoC for
// axios@1.12.0 is about to land — who pages out?" Phase 1 does an O(IoCs
// × SBOMs) scan; Phase 2's bitmap index turns this into O(|components|).
func (s *Server) BlastRadius(w http.ResponseWriter, r *http.Request) {
	var body gen.BlastRadiusRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}
	if len(body.Iocs) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "iocs is required and must be non-empty")
		return
	}

	comps, err := s.storage.ListComponents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	affected := map[string]struct{}{}
	for _, genIoC := range body.Iocs {
		ioc := fromGenIoC(genIoC)
		if err := ioc.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_IOC", err.Error())
			return
		}
		for _, c := range comps {
			sboms, err := s.storage.ListSBOMsByComponent(r.Context(), c.Ref)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
				return
			}
			for _, sbom := range sboms {
				if matcher.Evaluate(ioc, sbom).Matched {
					affected[c.Ref] = struct{}{}
					break
				}
			}
		}
	}
	out := make([]string, 0, len(affected))
	for ref := range affected {
		out = append(out, ref)
	}
	sort.Strings(out)
	writeJSON(w, http.StatusOK, gen.BlastRadiusResponse{
		AffectedComponentRefs: out,
		ComputedAt:            time.Now().UTC(),
	})
}

func toGenIncident(inc domain.Incident) gen.Incident {
	out := gen.Incident{
		Id:                 inc.ID,
		IocId:              inc.IoCID,
		State:              gen.IncidentState(inc.State),
		OpenedAt:           inc.OpenedAt,
		LastTransitionedAt: inc.LastTransitionedAt,
	}
	if len(inc.AffectedComponentsSnapshot) > 0 {
		snap := inc.AffectedComponentsSnapshot
		out.AffectedComponentsSnapshot = &snap
	}
	if len(inc.Remediations) > 0 {
		rs := make([]gen.Remediation, len(inc.Remediations))
		for i, r := range inc.Remediations {
			rs[i] = toGenRemediation(r)
		}
		out.Remediations = &rs
	}
	return out
}

// toGenRemediation is declared here so the Incident hydrator can reach
// it; the full remediation handler lives in handlers_remediation.go.
func toGenRemediation(r domain.Remediation) gen.Remediation {
	out := gen.Remediation{
		Id:         r.ID,
		IncidentId: r.IncidentID,
		Kind:       gen.RemediationKind(r.Kind),
		ExecutedAt: r.ExecutedAt,
	}
	if r.ActorRef != "" {
		a := r.ActorRef
		out.ActorRef = &a
	}
	if len(r.Details) > 0 {
		d := map[string]any(r.Details)
		out.Details = &d
	}
	return out
}
