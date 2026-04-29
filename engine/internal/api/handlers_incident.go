package api

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// ListIncidents returns incidents filtered by state / ecosystem / since.
// Sorted by OpenedAt desc (newest first — the Backstage IncidentDashboard
// reads the list top-down and operators look at the most recent first).
// Cursor pagination is deferred (no specific theme yet); the params
// are accepted and ignored so clients that already send them don't
// break.
func (s *Server) ListIncidents(w http.ResponseWriter, r *http.Request, params gen.ListIncidentsParams) {
	filter := domain.IncidentFilter{}
	if params.State != nil {
		for _, st := range *params.State {
			filter.States = append(filter.States, domain.IncidentState(st))
		}
	}
	if params.Ecosystem != nil {
		for _, eco := range *params.Ecosystem {
			if eco != "" {
				filter.Ecosystems = append(filter.Ecosystems, eco)
			}
		}
	}
	// `from` wins when both `from` and `since` are supplied — `since`
	// is the v0.2.0-era alias preserved for backward compat.
	switch {
	case params.From != nil:
		t := *params.From
		filter.From = &t
	case params.Since != nil:
		t := *params.Since
		filter.From = &t
	}
	if params.To != nil {
		t := *params.To
		filter.To = &t
	}
	if params.Search != nil {
		filter.Search = *params.Search
	}
	if params.Owner != nil {
		filter.Owner = *params.Owner
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}

	incs, err := s.storage.ListIncidentsFiltered(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	items := make([]gen.Incident, 0, len(incs))
	for _, inc := range incs {
		items = append(items, toGenIncident(inc))
	}
	writeJSON(w, http.StatusOK, gen.IncidentPage{Items: items})
}

// GetIncident returns a single incident. Snapshot + remediations come
// back as-is; the linked IoC is not hydrated today — clients who need
// IoC detail hit /v1/iocs?… themselves. That's a deliberate trade-off
// (keeps the handler stateless; no N+1 joins in memory storage).
// A future denormalised view could move the join into the response
// if the UI proves it's a hot path; not currently scoped.
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

// GetIncidentDetail backs the IncidentDetailDrawer in the Backstage
// frontend with a single round-trip: the incident row + its IoC + every
// component referenced by AffectedComponentsSnapshot, all hydrated.
// Remediations are already part of the Incident value (append-only on
// the same row), so no separate ListRemediations call is needed.
//
// Performance posture: 4-N storage calls (1 GetIncident + 1 GetIoC + N
// GetComponent for N affected components). Memory backend keeps these
// under a millisecond; postgres pool services them serially in a single
// HTTP request — well inside the <200ms drawer-open budget for typical
// incidents (≤10 components). N+1 across thousands of components would
// need a denormalised view; not the v0.2.0 traffic shape.
//
// Failure-mode discipline: missing IoC or missing component refs no
// longer 404 the whole detail call (incident history must survive
// catalog churn). The IoC field is omitted when the IoC has been
// deleted; affected components silently drop deleted refs.
func (s *Server) GetIncidentDetail(w http.ResponseWriter, r *http.Request, id string) {
	inc, err := s.storage.GetIncident(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "incident "+id+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	out := gen.IncidentDetail{
		Incident:           toGenIncident(*inc),
		AffectedComponents: &[]gen.Component{},
	}

	if inc.IoCID != "" {
		ioc, err := s.storage.GetIoC(r.Context(), inc.IoCID)
		if err == nil && ioc != nil {
			gioc := toGenIoC(*ioc)
			out.Ioc = &gioc
		}
	}

	if len(inc.AffectedComponentsSnapshot) > 0 {
		hydrated := make([]gen.Component, 0, len(inc.AffectedComponentsSnapshot))
		for _, ref := range inc.AffectedComponentsSnapshot {
			c, err := s.storage.GetComponent(r.Context(), ref)
			if err != nil || c == nil {
				continue
			}
			hydrated = append(hydrated, toGenComponent(*c))
		}
		out.AffectedComponents = &hydrated
	}

	writeJSON(w, http.StatusOK, out)
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
// without opening incidents. Hybrid lookup:
//
//   - Cache path: an IoC already ingested into the engine has rows in
//     the incidents table for every (IoC, component) pair the matcher
//     fired on at submit time. We just SELECT them out — single
//     indexed query, no live re-scan.
//   - Live path: an IoC the caller is asking about hypothetically
//     ("what if axios@1.12.0 dropped right now?") has no incidents.
//     We fall back to a single bulk lookup over sbom_packages by
//     (ecosystem, name) and run matcher.Evaluate over the
//     candidates.
//
// Both paths produce the same response shape. The what-if contract is
// covered by TestBlastRadius_ReturnsAffectedComponents (hypothetical
// IoC ID 01IOC-HYPO); the cache path by
// TestBlastRadius_CachedPathForIngestedIoC.
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

	ctx := r.Context()
	affected := map[string]struct{}{}
	for _, genIoC := range body.Iocs {
		ioc := fromGenIoC(genIoC)
		if err := ioc.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_IOC", err.Error())
			return
		}

		// Cache-first: was this IoC already ingested? GetIoC
		// distinguishes a real storage error from a "never seen"
		// miss via storage.ErrNotFound.
		_, err := s.storage.GetIoC(ctx, ioc.ID)
		switch {
		case err == nil:
			refs, err := s.storage.MatchedComponentRefsByIoC(ctx, ioc.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
				return
			}
			for _, ref := range refs {
				affected[ref] = struct{}{}
			}
			continue
		case errors.Is(err, storage.ErrNotFound):
			// Fall through to live path.
		default:
			writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
			return
		}

		// Live path (hypothetical IoC).
		eco, name, ok := iocLookupKey(ioc)
		if !ok {
			// IoC variant carries no (ecosystem, name) hook — no
			// candidates to evaluate, contributes nothing.
			continue
		}
		pkgs, err := s.storage.ListSBOMPackages(ctx, eco, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
			return
		}
		for _, ref := range matchPackagesAgainstIoC(ioc, eco, pkgs) {
			affected[ref] = struct{}{}
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
