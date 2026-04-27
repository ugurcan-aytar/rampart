package maintainerdrift

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// fixedNow returns a Detector clocked to t — convenient for stable
// confidence-window assertions that don't depend on wall time.
func fixedNow(t time.Time) *Detector {
	d := New()
	d.now = func() time.Time { return t }
	return d
}

func mkSnap(t time.Time, latestVer string, latestPub *time.Time, maintEmails ...string) domain.PublisherSnapshot {
	ms := make([]domain.Maintainer, 0, len(maintEmails))
	for _, e := range maintEmails {
		ms = append(ms, domain.Maintainer{Email: e})
	}
	return domain.PublisherSnapshot{
		PackageRef:               "npm:axios",
		SnapshotAt:               t,
		Maintainers:              ms,
		LatestVersion:            latestVer,
		LatestVersionPublishedAt: latestPub,
	}
}

func TestDetect_NoAnomalyOnSingleSnapshot(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	hist := []domain.PublisherSnapshot{mkSnap(now, "1.0.0", &now, "a@example.com")}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "single snapshot has no baseline → no anomaly")
}

func TestDetect_NoAnomalyWhenMaintainersUnchanged(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	pub := now.Add(-1 * time.Hour)
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &pub, "a@example.com"),                   // newest
		mkSnap(now.Add(-2*time.Hour), "1.0.0", &pub, "a@example.com"), // baseline
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetect_HighConfidence_NewEmailRecentPublishFirstSeen(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-3 * time.Hour) // well within 7d
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &publish, "a@example.com", "evil@bad.io"), // newest: evil added
		mkSnap(now.Add(-1*time.Hour), "1.0.0", nil, "a@example.com"),
		mkSnap(now.Add(-2*time.Hour), "1.0.0", nil, "a@example.com"), // baseline
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceHigh, got[0].Confidence)
	require.Equal(t, domain.AnomalyKindMaintainerEmailDrift, got[0].Kind)
	require.Contains(t, got[0].Evidence["new_emails"].([]string), "evil@bad.io")
}

func TestDetect_MediumConfidence_NewEmailMidWindow(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-10 * 24 * time.Hour) // 10 days = medium window
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &publish, "a@example.com", "newer@example.com"),
		mkSnap(now.Add(-1*time.Hour), "1.0.0", &publish, "a@example.com"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceMedium, got[0].Confidence)
}

func TestDetect_LowConfidence_NewEmailButOldPublish(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-60 * 24 * time.Hour) // 60 days
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.0", &publish, "a@example.com", "newish@example.com"),
		mkSnap(now.Add(-1*time.Hour), "1.0.0", &publish, "a@example.com"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceLow, got[0].Confidence)
}

func TestDetect_LowConfidence_WhenLatestPublishUnknown(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.0", nil, "a@example.com", "x@example.com"), // no publish ts
		mkSnap(now.Add(-1*time.Hour), "1.0.0", nil, "a@example.com"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceLow, got[0].Confidence,
		"unknown publish time prevents High classification")
}

func TestDetect_CaseInsensitiveEmailComparison(t *testing.T) {
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-1 * time.Hour)
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &publish, "Alice@Example.COM"), // same email, different case
		mkSnap(now.Add(-1*time.Hour), "1.0.0", nil, "alice@example.com"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "case differences must NOT trigger drift")
}

func TestDetect_DowngradesHighWhenEmailWasSeenInDeeperHistory(t *testing.T) {
	// New email today, but the same address appeared in mid-history —
	// likely a flaky registry response, not a takeover. Stay below High.
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-2 * time.Hour) // recent publish
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &publish, "a@example.com", "x@example.com"),              // newest: x re-added
		mkSnap(now.Add(-1*time.Hour), "1.0.1", nil, "a@example.com"),                  // baseline (x missing)
		mkSnap(now.Add(-2*time.Hour), "1.0.0", nil, "a@example.com", "x@example.com"), // x WAS here earlier
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotEqual(t, domain.ConfidenceHigh, got[0].Confidence,
		"address that re-appears (was in deeper history) is not first-seen → not High")
}

func TestDetect_AddedMaintainerCountsLikeChange(t *testing.T) {
	// "Joining the team" and "taking over" are indistinguishable from
	// the registry's perspective — both are new addresses with
	// publish access. The detector treats them the same.
	now := time.Now().UTC()
	d := fixedNow(now)
	publish := now.Add(-1 * time.Hour)
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "1.0.1", &publish, "a@example.com", "b@example.com", "newjoin@example.com"),
		mkSnap(now.Add(-1*time.Hour), "1.0.0", nil, "a@example.com", "b@example.com"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceHigh, got[0].Confidence)
}
