package api

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/matcher"
)

// isNonTerminal reports whether an incident is still actionable. Closed
// and Dismissed are terminal: a fresh match against the same (IoC, ref)
// opens a new incident rather than reviving the dead one.
func isNonTerminal(state domain.IncidentState) bool {
	return state != domain.StateClosed && state != domain.StateDismissed
}

// openIncidentsForMatches is the shared path used by both SubmitSBOM
// (retroactive: match one new SBOM against all IoCs) and SubmitIoC
// (forward: match one new IoC against all SBOMs). For every unique
// matched (IoC, componentRef) pair that doesn't already have an open
// incident, it creates one and publishes `incident.opened`. Returns the
// list of newly-opened IncidentIDs.
//
// Idempotency: keyed by (IoCID, ComponentRef). If an open incident
// already exists for that pair, this call is a no-op for that pair — we
// do NOT re-emit `incident.opened` (the caller has already seen it).
// Terminal incidents (closed / dismissed) do not block a new one; a
// recurrence deserves a fresh event.
func (s *Server) openIncidentsForMatches(ctx context.Context, pairs []matchPair) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	existing, err := s.storage.ListIncidents(ctx)
	if err != nil {
		return nil, err
	}
	// Build an index: (IoCID, ComponentRef) -> non-terminal incident.
	type key struct{ ioc, ref string }
	open := map[key]domain.Incident{}
	for _, inc := range existing {
		if !isNonTerminal(inc.State) {
			continue
		}
		for _, r := range inc.AffectedComponentsSnapshot {
			open[key{inc.IoCID, r}] = inc
		}
	}

	now := time.Now().UTC()
	var opened []string
	for _, p := range pairs {
		if _, already := open[key{p.IoCID, p.ComponentRef}]; already {
			continue
		}
		incident := domain.Incident{
			ID:                         ulid.Make().String(),
			IoCID:                      p.IoCID,
			State:                      domain.StatePending,
			OpenedAt:                   now,
			LastTransitionedAt:         now,
			AffectedComponentsSnapshot: []string{p.ComponentRef},
		}
		if err := s.storage.UpsertIncident(ctx, incident); err != nil {
			return opened, err
		}
		s.events.Publish(domain.IncidentOpenedEvent{
			IncidentID:                 incident.ID,
			IoCID:                      incident.IoCID,
			AffectedComponentsSnapshot: incident.AffectedComponentsSnapshot,
			At:                         now,
		})
		opened = append(opened, incident.ID)
		// Remember it for the rest of this call so we don't open two
		// incidents for the same (ioc, ref) pair if `pairs` happens to
		// duplicate.
		open[key{p.IoCID, p.ComponentRef}] = incident
	}
	return opened, nil
}

// matchPair is a single (IoC, affected component) hit flattened out for
// openIncidentsForMatches. ComponentRef comes from the SBOM the IoC
// matched against — an SBOM belongs to exactly one Component.
type matchPair struct {
	IoCID        string
	ComponentRef string
}

// retroactiveMatch walks every stored IoC and returns the pairs that
// fire against `sbom`. Called from SubmitSBOM after a new SBOM is
// persisted. Duplicates (same IoCID) are collapsed.
func (s *Server) retroactiveMatch(ctx context.Context, sbom domain.SBOM) ([]matchPair, []string, error) {
	iocs, err := s.storage.ListIoCs(ctx)
	if err != nil {
		return nil, nil, err
	}
	var pairs []matchPair
	matchedIoCs := make(map[string]struct{}, len(iocs))
	for _, ioc := range iocs {
		if !matcher.Evaluate(ioc, sbom).Matched {
			continue
		}
		if _, dup := matchedIoCs[ioc.ID]; dup {
			continue
		}
		matchedIoCs[ioc.ID] = struct{}{}
		pairs = append(pairs, matchPair{IoCID: ioc.ID, ComponentRef: sbom.ComponentRef})
	}
	iocIDs := make([]string, 0, len(matchedIoCs))
	for id := range matchedIoCs {
		iocIDs = append(iocIDs, id)
	}
	return pairs, iocIDs, nil
}

// forwardMatch walks every stored SBOM and returns the pairs that fire
// against `ioc`. Called from SubmitIoC after a new IoC is persisted.
// Dedupes by ComponentRef: if a component has multiple historical
// SBOMs (new commits over time), only the first match produces a pair.
func (s *Server) forwardMatch(ctx context.Context, ioc domain.IoC) ([]matchPair, []string, error) {
	comps, err := s.storage.ListComponents(ctx)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]struct{}{}
	var pairs []matchPair
	var matchedComponents []string
	for _, c := range comps {
		sboms, err := s.storage.ListSBOMsByComponent(ctx, c.Ref)
		if err != nil {
			return nil, nil, err
		}
		for _, sbom := range sboms {
			if !matcher.Evaluate(ioc, sbom).Matched {
				continue
			}
			if _, dup := seen[c.Ref]; dup {
				continue
			}
			seen[c.Ref] = struct{}{}
			pairs = append(pairs, matchPair{IoCID: ioc.ID, ComponentRef: c.Ref})
			matchedComponents = append(matchedComponents, c.Ref)
			break // one SBOM is enough to pin the blast radius for this component
		}
	}
	return pairs, matchedComponents, nil
}
