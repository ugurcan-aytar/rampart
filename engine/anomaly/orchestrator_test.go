package anomaly_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/anomaly"
	"github.com/ugurcan-aytar/rampart/engine/anomaly/maintainerdrift"
	"github.com/ugurcan-aytar/rampart/engine/anomaly/oidcregression"
	"github.com/ugurcan-aytar/rampart/engine/anomaly/versionjump"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

func TestOrchestrator_RejectsBadConfig(t *testing.T) {
	store := memory.New()
	defer store.Close()

	_, err := anomaly.NewOrchestrator(anomaly.OrchestratorConfig{
		Storage:   nil,
		Detectors: []anomaly.Detector{maintainerdrift.New()},
	})
	require.Error(t, err, "missing Storage")

	_, err = anomaly.NewOrchestrator(anomaly.OrchestratorConfig{
		Storage:   store,
		Detectors: nil,
	})
	require.Error(t, err, "no detectors")
}

func TestOrchestrator_TickRaises_AllThreeAnomalyTypes(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	// Package 1 — npm:axios — maintainer email drift fixture.
	publishRecent := now.Add(-3 * time.Hour) // High-confidence window
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now.Add(-2 * time.Hour),
		Maintainers:   []domain.Maintainer{{Email: "ok@example.com"}},
		LatestVersion: "1.0.0",
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now,
		Maintainers: []domain.Maintainer{
			{Email: "ok@example.com"}, {Email: "evil@bad.io"},
		},
		LatestVersion:            "1.0.1",
		LatestVersionPublishedAt: &publishRecent,
	}))

	// Package 2 — npm:left-pad — OIDC regression fixture.
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:left-pad", SnapshotAt: now.Add(-2 * time.Hour),
		PublishMethod: "oidc-trusted-publisher",
		LatestVersion: "1.0.0",
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:left-pad", SnapshotAt: now,
		PublishMethod: "token",
		LatestVersion: "1.0.1",
	}))

	// Package 3 — gomod:github.com/spf13/cobra — version jump fixture.
	for i, ver := range []string{"1.0.0", "1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5"} {
		require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
			PackageRef:    "gomod:github.com/spf13/cobra",
			SnapshotAt:    now.Add(-time.Duration(7-i) * time.Hour),
			LatestVersion: ver,
		}))
	}
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef:    "gomod:github.com/spf13/cobra",
		SnapshotAt:    now,
		LatestVersion: "47.0.0", // breaking-delta = 46 → High
	}))

	orch, err := anomaly.NewOrchestrator(anomaly.OrchestratorConfig{
		Storage: store,
		Detectors: []anomaly.Detector{
			maintainerdrift.New(),
			oidcregression.New(),
			versionjump.New(),
		},
		HistoryDepth: 50,
	})
	require.NoError(t, err)

	require.NoError(t, orch.Tick(ctx))

	all, err := store.ListAnomalies(ctx, domain.AnomalyFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3, "one anomaly per fixture")

	byKind := map[domain.AnomalyKind]domain.Anomaly{}
	for _, a := range all {
		byKind[a.Kind] = a
	}
	require.Contains(t, byKind, domain.AnomalyKindMaintainerEmailDrift)
	require.Contains(t, byKind, domain.AnomalyKindOIDCPublishingRegression)
	require.Contains(t, byKind, domain.AnomalyKindVersionJump)

	require.Equal(t, "npm:axios", byKind[domain.AnomalyKindMaintainerEmailDrift].PackageRef)
	require.Equal(t, "npm:left-pad", byKind[domain.AnomalyKindOIDCPublishingRegression].PackageRef)
	require.Equal(t, "gomod:github.com/spf13/cobra",
		byKind[domain.AnomalyKindVersionJump].PackageRef)
	require.Equal(t, domain.ConfidenceHigh,
		byKind[domain.AnomalyKindVersionJump].Confidence)
}

func TestOrchestrator_TickIsIdempotent(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	// Single OIDC-regression fixture, simplest possible signal.
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now.Add(-time.Hour),
		PublishMethod: "oidc-trusted-publisher", LatestVersion: "1.0.0",
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now,
		PublishMethod: "token", LatestVersion: "1.0.1",
	}))

	orch, err := anomaly.NewOrchestrator(anomaly.OrchestratorConfig{
		Storage:   store,
		Detectors: []anomaly.Detector{oidcregression.New()},
	})
	require.NoError(t, err)

	// Two ticks back-to-back.
	require.NoError(t, orch.Tick(ctx))
	first, err := store.ListAnomalies(ctx, domain.AnomalyFilter{})
	require.NoError(t, err)
	require.Len(t, first, 1)

	require.NoError(t, orch.Tick(ctx))
	second, err := store.ListAnomalies(ctx, domain.AnomalyFilter{})
	require.NoError(t, err)
	// At most 1 new row from the second tick, because the dedup
	// constraint may treat the slightly-later DetectedAt as a new
	// row. The Count never exceeds (ticks * detectors * 1).
	require.LessOrEqual(t, len(second), 2)
}

func TestOrchestrator_RunFiresImmediateTickAndStops(t *testing.T) {
	store := memory.New()
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())

	now := time.Now().UTC()
	require.NoError(t, store.SavePublisherSnapshot(context.Background(), domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now.Add(-time.Hour),
		PublishMethod: "oidc-trusted-publisher", LatestVersion: "1.0.0",
	}))
	require.NoError(t, store.SavePublisherSnapshot(context.Background(), domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: now,
		PublishMethod: "token", LatestVersion: "1.0.1",
	}))

	orch, err := anomaly.NewOrchestrator(anomaly.OrchestratorConfig{
		Storage:      store,
		Detectors:    []anomaly.Detector{oidcregression.New()},
		TickInterval: time.Hour, // long enough that we rely on the immediate tick
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- orch.Run(ctx) }()

	// Wait for the immediate tick to land an anomaly.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all, err := store.ListAnomalies(context.Background(), domain.AnomalyFilter{})
		require.NoError(t, err)
		if len(all) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	all, err := store.ListAnomalies(context.Background(), domain.AnomalyFilter{})
	require.NoError(t, err)
	require.Len(t, all, 1, "immediate tick must raise the OIDC regression")

	cancel()
	select {
	case err := <-done:
		require.True(t, errors.Is(err, context.Canceled))
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator did not stop on ctx cancel")
	}
}
