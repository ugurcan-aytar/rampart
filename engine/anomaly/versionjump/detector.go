// Package versionjump detects releases that deviate from a package's
// established cadence: a project that's been bumping patch / minor for
// the last N snapshots and suddenly publishes a multi-major jump.
// Mass-breaking-change releases are unusual and worth surfacing.
//
// "Breaking delta" is the unit. For ≥1.0 packages it's the major
// number diff; for 0.x packages it's the minor number diff (in 0.x
// semver, minor IS the breaking-change axis). The 0.x → 1.x
// transition counts as a single bump (going stable is expected).
//
// Confidence model:
//   - High: 5+ historical snapshots with breaking delta == 0
//     (consistent patch cadence) AND latest breaking delta ≥ 5.
//   - Medium: 3-4 historical snapshots mostly at delta 0/1 AND
//     latest delta 2-4.
//   - Low: fewer than 3 historical jumps but any latest delta ≥ 2.
//
// Edge cases handled:
//   - Pre-release versions (`1.0.0-rc.1`): dropped before comparison;
//     `1.0.0-rc.1 → 1.0.0` is not a jump.
//   - Downgrade (`2.0.0 → 1.0.0`): negative delta, ignored.
//   - Same version repeated (ingestor noise): collapsed before
//     computing jumps.
//   - Unparseable semver: dropped with no anomaly (do not crash on
//     `git+https://…` or other registry quirks).
//   - History of fewer than 2 distinct versions: nothing to compare,
//     return nil.
package versionjump

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Detector implements [anomaly.Detector] for the version-jump class.
type Detector struct {
	now func() time.Time
}

// New returns a Detector with the wall-clock as `now`.
func New() *Detector {
	return &Detector{now: func() time.Time { return time.Now().UTC() }}
}

// Kind reports the detector's anomaly class.
func (d *Detector) Kind() domain.AnomalyKind {
	return domain.AnomalyKindVersionJump
}

// Detect implements [anomaly.Detector].
func (d *Detector) Detect(_ context.Context, packageRef string, history []domain.PublisherSnapshot) ([]domain.Anomaly, error) {
	// History arrives newest-first; we want oldest-to-newest for the
	// chronological walk.
	versions := extractVersionsOldestFirst(history)
	if len(versions) < 2 {
		return nil, nil
	}

	// Compute jumps oldest → newest. The LATEST jump is the one we're
	// classifying; everything before it is the historical baseline.
	jumps := make([]int64, 0, len(versions)-1)
	for i := 1; i < len(versions); i++ {
		jumps = append(jumps, breakingDelta(versions[i-1], versions[i]))
	}
	latestJump := jumps[len(jumps)-1]
	if latestJump <= 0 {
		// Downgrade or no-op: out of scope for this detector.
		return nil, nil
	}
	historical := jumps[:len(jumps)-1]

	if !isUnusualVsHistory(latestJump, historical) {
		return nil, nil
	}

	conf := classify(latestJump, historical)

	prev := versions[len(versions)-2]
	curr := versions[len(versions)-1]
	expl := fmt.Sprintf(
		"%s jumped from %s to %s — breaking-delta %d vs historical max %d over %d prior jump(s)",
		packageRef, prev.Original(), curr.Original(),
		latestJump, maxOrZero(historical), len(historical),
	)
	evidence := map[string]any{
		"previous_version":      prev.Original(),
		"current_version":       curr.Original(),
		"breaking_delta":        latestJump,
		"historical_max_delta":  maxOrZero(historical),
		"historical_jump_count": len(historical),
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

// extractVersionsOldestFirst returns parsed semvers in chronological
// order, dropping unparseable versions and pre-releases, then collapses
// consecutive duplicates (the ingestor sees the same version on
// successive ticks until a new release lands).
func extractVersionsOldestFirst(history []domain.PublisherSnapshot) []*semver.Version {
	out := make([]*semver.Version, 0, len(history))
	// Iterate newest-first list in REVERSE → oldest first.
	for i := len(history) - 1; i >= 0; i-- {
		raw := history[i].LatestVersion
		if raw == "" {
			continue
		}
		v, err := semver.NewVersion(raw)
		if err != nil {
			continue
		}
		if v.Prerelease() != "" {
			// `1.0.0-rc.1` shouldn't anchor a "stable cadence"; the
			// downstream → `1.0.0` transition would otherwise look
			// like a jump.
			continue
		}
		// Collapse consecutive duplicates.
		if len(out) > 0 && out[len(out)-1].Equal(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// breakingDelta returns the magnitude of the breaking-change axis
// between two consecutive versions:
//   - both ≥ 1.0: major delta
//   - both 0.x:   minor delta
//   - 0.x → ≥1.0: 1 (the going-stable event, expected and small)
//
// Negative results indicate a downgrade and the caller should treat
// them as "not a jump".
func breakingDelta(prev, curr *semver.Version) int64 {
	// semver.Version exposes Major/Minor as uint64. Real-world package
	// versions never approach int64 max, so the narrowing conversion is
	// safe; gosec doesn't see the practical bound.
	prevMajor := int64(prev.Major()) //nolint:gosec // G115: bounded by real-world version numbers
	currMajor := int64(curr.Major()) //nolint:gosec // G115: bounded by real-world version numbers
	switch {
	case prevMajor == 0 && currMajor == 0:
		return int64(curr.Minor()) - int64(prev.Minor()) //nolint:gosec // G115: bounded by real-world version numbers
	case prevMajor == 0 && currMajor >= 1:
		// 0.x → 1.x. Treat the going-stable event as a single bump
		// regardless of the actual major number on the right; semver
		// supports skipping straight to 1.0 OR jumping to 2.0+ from 0.x.
		return 1
	default:
		return currMajor - prevMajor
	}
}

// isUnusualVsHistory returns true when latestJump materially exceeds
// what the historical jumps suggested. Two cases qualify:
//   - latestJump ≥ 2 (an outright big bump — at minimum medium-worthy).
//   - latestJump > 2 × max(historical) (a genuine outlier even when
//     historical baseline already had small jumps).
func isUnusualVsHistory(latestJump int64, historical []int64) bool {
	if latestJump < 2 {
		return false
	}
	histMax := maxOrZero(historical)
	return latestJump >= 2 && latestJump > 2*histMax
}

// classify assigns a confidence level based on history depth and
// the magnitude of the latest jump.
func classify(latestJump int64, historical []int64) domain.ConfidenceLevel {
	patchOrMinor := 0
	for _, j := range historical {
		if j == 0 {
			patchOrMinor++
		}
	}
	switch {
	case patchOrMinor >= 5 && latestJump >= 5:
		return domain.ConfidenceHigh
	case len(historical) >= 2 && latestJump >= 2:
		return domain.ConfidenceMedium
	default:
		return domain.ConfidenceLow
	}
}

func maxOrZero(xs []int64) int64 {
	var m int64
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}
