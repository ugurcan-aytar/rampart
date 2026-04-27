// Package iocforward is the shared upsert-then-match path called by both
// the api layer (`/v1/iocs` POST handler) and the anomaly orchestrator
// (Theme F2.3, post-SaveAnomaly). Both produce IoCs and want the
// "match against every stored SBOM, open an incident per fresh hit"
// behaviour; this package owns it so the orchestrator doesn't have to
// depend on the api package (ADR-0014).
package iocforward

import (
	"context"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/matcher"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Result reports what Submit did. Useful for callers that publish their
// own follow-up events (the api layer publishes ioc.matched after a
// successful Submit; the orchestrator just logs).
type Result struct {
	OpenedIncidents   []string
	MatchedComponents []string
}

// Submit upserts the IoC, walks every stored SBOM, opens one incident
// per fresh (IoC, ComponentRef) match, and publishes
// `incident.opened` for each. Idempotent: re-Submitting the same IoC
// against components that already have an open incident is a no-op
// for those components (the dedup key is (IoCID, ComponentRef) on
// non-terminal incidents).
//
// Validation lives outside this package — call sites are expected to
// have already run `domain.IoC.Validate()` and any kind-specific
// pre-checks (e.g. semver constraint parse for PackageRange).
func Submit(ctx context.Context, store storage.Storage, bus *events.Bus, ioc domain.IoC) (Result, error) {
	if err := store.UpsertIoC(ctx, ioc); err != nil {
		return Result{}, fmt.Errorf("iocforward: upsert: %w", err)
	}
	pairs, matched, err := forwardMatch(ctx, store, ioc)
	if err != nil {
		return Result{}, fmt.Errorf("iocforward: match: %w", err)
	}
	opened, err := openIncidentsForMatches(ctx, store, bus, pairs)
	if err != nil {
		return Result{OpenedIncidents: opened, MatchedComponents: matched},
			fmt.Errorf("iocforward: open: %w", err)
	}
	if bus != nil && len(matched) > 0 {
		bus.Publish(domain.IoCMatchedEvent{
			IoCID:             ioc.ID,
			MatchedComponents: matched,
			At:                time.Now().UTC(),
		})
	}
	return Result{OpenedIncidents: opened, MatchedComponents: matched}, nil
}

type matchPair struct {
	IoCID        string
	ComponentRef string
}

// forwardMatch is the same algorithm api.matching.go used pre-extract:
// walk every component, run the matcher against each of its SBOMs,
// emit one pair per fresh (IoC, ComponentRef) hit. Dedupes by
// ComponentRef so historical SBOMs don't produce duplicate pairs.
func forwardMatch(ctx context.Context, store storage.Storage, ioc domain.IoC) ([]matchPair, []string, error) {
	comps, err := store.ListComponents(ctx)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]struct{}{}
	var pairs []matchPair
	var matched []string
	for _, c := range comps {
		sboms, err := store.ListSBOMsByComponent(ctx, c.Ref)
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
			matched = append(matched, c.Ref)
			break
		}
	}
	return pairs, matched, nil
}

// isNonTerminal reports whether an incident is still actionable. Closed
// and Dismissed are terminal: a fresh match against the same (IoC, ref)
// opens a new incident rather than reviving the dead one.
func isNonTerminal(state domain.IncidentState) bool {
	return state != domain.StateClosed && state != domain.StateDismissed
}

// openIncidentsForMatches creates one incident per fresh
// (IoCID, ComponentRef) pair that doesn't already have a non-terminal
// incident. Mirrors api.matching.openIncidentsForMatches with one
// difference: bus may be nil (the orchestrator can choose not to
// publish), in which case events are skipped.
func openIncidentsForMatches(ctx context.Context, store storage.Storage, bus *events.Bus, pairs []matchPair) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	existing, err := store.ListIncidents(ctx)
	if err != nil {
		return nil, err
	}
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
		if err := store.UpsertIncident(ctx, incident); err != nil {
			return opened, err
		}
		if bus != nil {
			bus.Publish(domain.IncidentOpenedEvent{
				IncidentID:                 incident.ID,
				IoCID:                      incident.IoCID,
				AffectedComponentsSnapshot: incident.AffectedComponentsSnapshot,
				At:                         now,
			})
		}
		opened = append(opened, incident.ID)
		open[key{p.IoCID, p.ComponentRef}] = incident
	}
	return opened, nil
}
