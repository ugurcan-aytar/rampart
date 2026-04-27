package api

import (
	"context"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/matcher"
)

// iocLookupKey extracts the (ecosystem, name) pair the matcher's bulk
// hot path (storage.ListSBOMPackages) keys on. Returns ok=false for an
// IoC variant we cannot bulk-lookup — the caller should treat that as
// "no matches" rather than fall through to a slow scan.
//
// The three branches mirror matcher.Evaluate's switch:
//   - packageVersion / packageRange carry name explicitly
//   - publisherAnomaly's AnomalyBody encodes "<ecosystem>:<name>" in
//     PackageRef (per ADR-0014). The legacy maintainer-keyed
//     PublisherAnomaly variant has no package binding and is a no-op
//     in matcher.Evaluate today, so it returns ok=false here too.
func iocLookupKey(ioc domain.IoC) (ecosystem, name string, ok bool) {
	switch ioc.Kind {
	case domain.IoCKindPackageVersion:
		if ioc.PackageVersion == nil {
			return "", "", false
		}
		return ioc.Ecosystem, ioc.PackageVersion.Name, true
	case domain.IoCKindPackageRange:
		if ioc.PackageRange == nil {
			return "", "", false
		}
		return ioc.Ecosystem, ioc.PackageRange.Name, true
	case domain.IoCKindPublisherAnomaly:
		if ioc.AnomalyBody == nil {
			return "", "", false
		}
		ref := ioc.AnomalyBody.PackageRef
		idx := strings.Index(ref, ":")
		if idx <= 0 || idx == len(ref)-1 {
			return "", "", false
		}
		return ref[:idx], ref[idx+1:], true
	}
	return "", "", false
}

// matchPackagesAgainstIoC runs the IoC's predicate against every
// (component, package) pair returned by storage.ListSBOMPackages and
// returns the distinct component refs that fired. The matcher is
// re-used unchanged: each row is wrapped in a one-package synthetic
// SBOM so packageVersion / packageRange / anomalyBody all flow
// through the same evaluator.
func matchPackagesAgainstIoC(ioc domain.IoC, ecosystem string, pkgs []domain.SBOMPackageRef) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, p := range pkgs {
		if _, dup := seen[p.ComponentRef]; dup {
			continue
		}
		synthetic := domain.SBOM{
			Ecosystem: ecosystem,
			Packages:  []domain.PackageVersion{p.Package},
		}
		if !matcher.Evaluate(ioc, synthetic).Matched {
			continue
		}
		seen[p.ComponentRef] = struct{}{}
		out = append(out, p.ComponentRef)
	}
	return out
}

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

// forwardMatch returns the (component) pairs that fire against `ioc`.
// Called from SubmitIoC after a new IoC is persisted.
//
// Issues a single bulk lookup against sbom_packages by (ecosystem,
// name) instead of looping ListSBOMsByComponent across every
// component. matcher.Evaluate stays in memory — the bulk lookup just
// shrinks the candidate set from `every package in every SBOM` to
// `every (component, version) row that matches the IoC's
// (ecosystem, name)`. Dedupes by ComponentRef so a monorepo with
// multiple historical SBOMs still produces one pair per component.
func (s *Server) forwardMatch(ctx context.Context, ioc domain.IoC) ([]matchPair, []string, error) {
	eco, name, ok := iocLookupKey(ioc)
	if !ok {
		return nil, nil, nil
	}
	pkgs, err := s.storage.ListSBOMPackages(ctx, eco, name)
	if err != nil {
		return nil, nil, err
	}
	matchedComponents := matchPackagesAgainstIoC(ioc, eco, pkgs)
	pairs := make([]matchPair, 0, len(matchedComponents))
	for _, ref := range matchedComponents {
		pairs = append(pairs, matchPair{IoCID: ioc.ID, ComponentRef: ref})
	}
	return pairs, matchedComponents, nil
}
