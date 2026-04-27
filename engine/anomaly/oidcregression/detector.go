// Package oidcregression detects a credential-compromise indicator: a
// package that previously published with an OIDC trusted publisher
// has rolled back to plain token-based publishing. The trusted-
// publisher path requires sigstore proof and a registered identity
// provider; falling back to a token bypasses both.
//
// Confidence model (binary by design — no Low):
//   - High: the previous snapshot had publish_method =
//     "oidc-trusted-publisher" AND the latest is "token". Adjacent
//     regression is the strongest signal: we can pin the change to
//     a specific publish event.
//   - Medium: SOME prior snapshot had OIDC, but the regression is
//     not adjacent (intervening "unknown" or token snapshots). The
//     account did at least once know how to use the OIDC path; the
//     current state still warrants triage.
//
// Edge cases handled:
//   - History of length < 2 → no baseline, no anomaly.
//   - publish_method = "unknown" is treated as "not OIDC" — we don't
//     have evidence either way, so it never triggers regression by
//     itself, but it doesn't disqualify a deeper-history OIDC entry.
//   - Anything other than "token" on the newest snapshot → no
//     regression (a fresh OIDC publish is not a regression).
package oidcregression

import (
	"context"
	"fmt"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

const (
	publishMethodOIDC  = "oidc-trusted-publisher"
	publishMethodToken = "token"
)

// Detector implements [anomaly.Detector] for the OIDC-regression class.
type Detector struct {
	now func() time.Time
}

// New returns a Detector with the wall-clock as `now`.
func New() *Detector {
	return &Detector{now: func() time.Time { return time.Now().UTC() }}
}

// Kind reports the detector's anomaly class.
func (d *Detector) Kind() domain.AnomalyKind {
	return domain.AnomalyKindOIDCPublishingRegression
}

// Detect implements [anomaly.Detector].
func (d *Detector) Detect(_ context.Context, packageRef string, history []domain.PublisherSnapshot) ([]domain.Anomaly, error) {
	if len(history) < 2 {
		return nil, nil
	}
	newest := history[0]
	if newest.PublishMethod != publishMethodToken {
		// Newest is not token → no regression today regardless of
		// what was before (a fresh OIDC publish is not a regression).
		return nil, nil
	}

	previous := history[1]
	priorOIDCAtIndex := -1
	for i, snap := range history[1:] {
		if snap.PublishMethod == publishMethodOIDC {
			priorOIDCAtIndex = i + 1 // +1 because slice started at 1
			break
		}
	}
	if priorOIDCAtIndex < 0 {
		// No prior snapshot ever had OIDC — there's nothing to regress
		// from. The package may simply be a token-only publisher.
		return nil, nil
	}

	conf := domain.ConfidenceMedium
	if previous.PublishMethod == publishMethodOIDC {
		conf = domain.ConfidenceHigh
	}
	expl := fmt.Sprintf(
		"%s publish_method regressed from %q to %q (latest version %s)",
		packageRef, publishMethodOIDC, publishMethodToken, newest.LatestVersion,
	)
	evidence := map[string]any{
		"newest_publish_method":   string(newest.PublishMethod),
		"previous_publish_method": string(previous.PublishMethod),
		"latest_version":          newest.LatestVersion,
		"oidc_seen_at_index":      priorOIDCAtIndex, // 1 = adjacent, larger = older
	}
	return []domain.Anomaly{{
		Kind:        d.Kind(),
		PackageRef:  packageRef,
		DetectedAt:  d.now(),
		Confidence:  conf,
		Explanation: expl,
		Evidence:    evidence,
	}}, nil
}
