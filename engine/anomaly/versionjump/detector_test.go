package versionjump

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// snapshots returns a newest-first slice from the given chronological
// (oldest-first) version strings — matches storage.GetPublisherHistory's
// natural order.
func snapshots(versions ...string) []domain.PublisherSnapshot {
	now := time.Now().UTC()
	out := make([]domain.PublisherSnapshot, len(versions))
	for i, v := range versions {
		// Reverse chronologically: index 0 = newest = last version.
		out[len(versions)-1-i] = domain.PublisherSnapshot{
			PackageRef:    "npm:axios",
			SnapshotAt:    now.Add(time.Duration(i) * -time.Hour),
			LatestVersion: v,
		}
	}
	return out
}

func TestDetect_NoAnomaly_SingleVersion(t *testing.T) {
	d := New()
	got, err := d.Detect(context.Background(), "npm:axios", snapshots("1.0.0"))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetect_NoAnomaly_PatchCadence(t *testing.T) {
	d := New()
	hist := snapshots("1.0.0", "1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "patch cadence is not a jump")
}

func TestDetect_NoAnomaly_StandardMinorBumps(t *testing.T) {
	d := New()
	hist := snapshots("1.0.0", "1.1.0", "1.2.0", "1.3.0")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "minor cadence is not a major jump")
}

func TestDetect_HighConfidence_LongCadenceThenMajorJump(t *testing.T) {
	d := New()
	// 6 patch jumps establish "consistent patch cadence", then a 47-major bump.
	hist := snapshots(
		"1.0.0", "1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5", "1.0.6",
		"48.0.0", // breaking-delta = 47
	)
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceHigh, got[0].Confidence)
	require.Equal(t, domain.AnomalyKindVersionJump, got[0].Kind)
	require.EqualValues(t, 47, got[0].Evidence["breaking_delta"])
}

func TestDetect_MediumConfidence_ShortCadenceModerateJump(t *testing.T) {
	d := New()
	// 3 prior jumps, modest history, then a 3-major bump.
	hist := snapshots("1.0.0", "1.0.1", "1.1.0", "4.0.0") // breaking-delta = 3
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceMedium, got[0].Confidence)
	require.EqualValues(t, 3, got[0].Evidence["breaking_delta"])
}

func TestDetect_LowConfidence_LimitedHistory(t *testing.T) {
	d := New()
	// Only 2 versions, jump ≥ 2.
	hist := snapshots("1.0.0", "5.0.0") // breaking-delta = 4 but only 1 jump → low
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceLow, got[0].Confidence)
}

func TestDetect_Pre1_0_MinorIsBreaking(t *testing.T) {
	d := New()
	// 0.x cadence by minor, then a 0.50 jump.
	hist := snapshots(
		"0.1.0", "0.1.1", "0.1.2", "0.1.3", "0.1.4", "0.1.5", "0.1.6",
		"0.50.0", // pre-1.0: minor delta = 49
	)
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceHigh, got[0].Confidence,
		"0.x packages: minor delta is the breaking axis")
}

func TestDetect_NoAnomaly_Pre1_0_MajorBumpToStable(t *testing.T) {
	d := New()
	// 0.9.0 → 1.0.0 is the going-stable event, not a jump.
	hist := snapshots("0.8.0", "0.9.0", "1.0.0")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "going stable (0.x → 1.0) is expected, not a jump")
}

func TestDetect_NoAnomaly_PreReleaseDropped(t *testing.T) {
	d := New()
	// `1.0.0-rc.1` should be dropped; `1.0.0-rc.2 → 1.0.0` should
	// not look like a jump from `0.9.0` either.
	hist := snapshots("0.9.0", "1.0.0-rc.1", "1.0.0-rc.2", "1.0.0")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "pre-release tags must be ignored before delta computation")
}

func TestDetect_NoAnomaly_Downgrade(t *testing.T) {
	d := New()
	// 2.0.0 → 1.5.0 — definitely odd, but not what THIS detector covers.
	hist := snapshots("1.0.0", "2.0.0", "1.5.0")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "downgrade is out of scope for version_jump")
}

func TestDetect_NoAnomaly_RepeatedSameVersion(t *testing.T) {
	d := New()
	// The ingestor sees the same version on successive ticks until a
	// new release lands; collapsed before computing jumps.
	hist := snapshots("1.0.0", "1.0.0", "1.0.0", "1.0.1")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "successive same-version snapshots collapse")
}

func TestDetect_NoAnomaly_UnparseableVersionDropped(t *testing.T) {
	d := New()
	// `git+https://…` is not semver; dropped before delta computation.
	hist := snapshots("1.0.0", "git+https://example.com/x.git#abc", "1.0.1")
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "unparseable versions don't crash + don't trigger")
}
