package oidcregression

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func mkSnap(t time.Time, method, ver string) domain.PublisherSnapshot {
	return domain.PublisherSnapshot{
		PackageRef:    "npm:axios",
		SnapshotAt:    t,
		PublishMethod: method,
		LatestVersion: ver,
	}
}

func TestDetect_NoAnomalyOnSingleSnapshot(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{mkSnap(now, "token", "1.0.0")}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetect_NoRegression_NewestIsOIDC(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "oidc-trusted-publisher", "1.0.1"),
		mkSnap(now.Add(-1*time.Hour), "token", "1.0.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "OIDC on newest is an UPGRADE, not a regression")
}

func TestDetect_HighConfidence_AdjacentRegression(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "token", "1.0.1"),
		mkSnap(now.Add(-1*time.Hour), "oidc-trusted-publisher", "1.0.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceHigh, got[0].Confidence)
	require.Equal(t, domain.AnomalyKindOIDCPublishingRegression, got[0].Kind)
	require.Equal(t, 1, got[0].Evidence["oidc_seen_at_index"])
}

func TestDetect_MediumConfidence_NonAdjacentRegression(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	// OIDC two snapshots back, intermediate "unknown", current "token".
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "token", "1.0.2"),
		mkSnap(now.Add(-1*time.Hour), "unknown", "1.0.1"),
		mkSnap(now.Add(-2*time.Hour), "oidc-trusted-publisher", "1.0.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.ConfidenceMedium, got[0].Confidence)
	require.Equal(t, 2, got[0].Evidence["oidc_seen_at_index"])
}

func TestDetect_NoRegression_NeverWasOIDC(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "token", "1.0.1"),
		mkSnap(now.Add(-1*time.Hour), "token", "1.0.0"),
		mkSnap(now.Add(-2*time.Hour), "unknown", "0.9.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "no prior OIDC → no regression")
}

func TestDetect_NoRegression_NewestIsUnknown(t *testing.T) {
	// "unknown" is not "token" — we can't say a regression happened
	// because we don't know what the current state actually is.
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "unknown", "1.0.1"),
		mkSnap(now.Add(-1*time.Hour), "oidc-trusted-publisher", "1.0.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetect_NoRegression_PriorIsUnknownNotOIDC(t *testing.T) {
	now := time.Now().UTC()
	d := New()
	hist := []domain.PublisherSnapshot{
		mkSnap(now, "token", "1.0.1"),
		mkSnap(now.Add(-1*time.Hour), "unknown", "1.0.0"),
	}
	got, err := d.Detect(context.Background(), "npm:axios", hist)
	require.NoError(t, err)
	require.Empty(t, got, "unknown→token is not regression — could be first measurement")
}
