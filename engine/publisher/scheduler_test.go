package publisher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
	"github.com/ugurcan-aytar/rampart/engine/publisher"
	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
)

// fakeIngestor records its calls and returns a canned response (or
// error) per call. Used for scheduler unit tests so we don't need a
// real httptest.Server.
type fakeIngestor struct {
	produce func(ctx context.Context, packageRef string) (*domain.PublisherSnapshot, error)
	calls   int
}

func (f *fakeIngestor) Ingest(ctx context.Context, ref string) (*domain.PublisherSnapshot, error) {
	f.calls++
	return f.produce(ctx, ref)
}

func TestScheduler_RejectsBadConfig(t *testing.T) {
	store := memory.New()
	defer store.Close()

	_, err := publisher.NewScheduler(publisher.SchedulerConfig{Storage: nil, Ingestors: map[string]publisher.Ingestor{"npm:": &fakeIngestor{}}})
	require.Error(t, err, "missing Storage")

	_, err = publisher.NewScheduler(publisher.SchedulerConfig{Storage: store, Ingestors: nil})
	require.Error(t, err, "no ingestors")

	_, err = publisher.NewScheduler(publisher.SchedulerConfig{
		Storage:   store,
		Ingestors: map[string]publisher.Ingestor{"npm": &fakeIngestor{}}, // missing trailing ':'
	})
	require.Error(t, err, "ingestor key must end with colon")
}

func TestScheduler_Tick_DispatchesByPrefix(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx := context.Background()

	// Seed two stale snapshots so ListPackagesNeedingRefresh returns
	// both refs.
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:axios", SnapshotAt: old,
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "gomod:github.com/spf13/cobra", SnapshotAt: old,
	}))

	npmIng := &fakeIngestor{produce: func(_ context.Context, ref string) (*domain.PublisherSnapshot, error) {
		return &domain.PublisherSnapshot{PackageRef: ref, LatestVersion: "1.11.0"}, nil
	}}
	ghIng := &fakeIngestor{produce: func(_ context.Context, ref string) (*domain.PublisherSnapshot, error) {
		return &domain.PublisherSnapshot{PackageRef: ref, LatestVersion: "v1.8.0"}, nil
	}}

	sch, err := publisher.NewScheduler(publisher.SchedulerConfig{
		Storage:         store,
		Ingestors:       map[string]publisher.Ingestor{"npm:": npmIng, "gomod:": ghIng},
		RefreshInterval: time.Hour, // makes anything older than 1h stale
		BatchSize:       10,
	})
	require.NoError(t, err)

	require.NoError(t, sch.Tick(ctx))

	require.Equal(t, 1, npmIng.calls, "npm ingestor must run once")
	require.Equal(t, 1, ghIng.calls, "gomod ingestor must run once")

	npmHist, err := store.GetPublisherHistory(ctx, "npm:axios", 0)
	require.NoError(t, err)
	require.Len(t, npmHist, 2, "fresh snapshot appended")
	require.Equal(t, "1.11.0", npmHist[0].LatestVersion)

	ghHist, err := store.GetPublisherHistory(ctx, "gomod:github.com/spf13/cobra", 0)
	require.NoError(t, err)
	require.Len(t, ghHist, 2)
	require.Equal(t, "v1.8.0", ghHist[0].LatestVersion)
}

func TestScheduler_Tick_SkipsRefRequiringNoIngestor(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx := context.Background()

	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "cargo:tokio", SnapshotAt: old,
	}))

	npmIng := &fakeIngestor{produce: func(_ context.Context, _ string) (*domain.PublisherSnapshot, error) {
		t.Fatal("npm ingestor must not be called for cargo: ref")
		return nil, nil
	}}
	sch, err := publisher.NewScheduler(publisher.SchedulerConfig{
		Storage:         store,
		Ingestors:       map[string]publisher.Ingestor{"npm:": npmIng},
		RefreshInterval: time.Hour,
	})
	require.NoError(t, err)

	require.NoError(t, sch.Tick(ctx))
	require.Equal(t, 0, npmIng.calls)
}

func TestScheduler_Tick_RateLimitedDoesNotAbortBatch(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx := context.Background()

	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:throttled", SnapshotAt: old,
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:succeeds", SnapshotAt: old,
	}))

	ing := &fakeIngestor{produce: func(_ context.Context, ref string) (*domain.PublisherSnapshot, error) {
		if ref == "npm:throttled" {
			return nil, httpx.ErrRateLimited
		}
		return &domain.PublisherSnapshot{PackageRef: ref, LatestVersion: "ok"}, nil
	}}
	sch, err := publisher.NewScheduler(publisher.SchedulerConfig{
		Storage:         store,
		Ingestors:       map[string]publisher.Ingestor{"npm:": ing},
		RefreshInterval: time.Hour,
	})
	require.NoError(t, err)

	require.NoError(t, sch.Tick(ctx))
	require.Equal(t, 2, ing.calls, "rate-limit on one ref must not stop the next")

	hist, err := store.GetPublisherHistory(ctx, "npm:succeeds", 0)
	require.NoError(t, err)
	require.Len(t, hist, 2, "successful ref must persist a fresh snapshot")
}

func TestScheduler_Run_FiresImmediateTickAndStopsOnContextCancel(t *testing.T) {
	store := memory.New()
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())

	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef: "npm:eager", SnapshotAt: old,
	}))

	called := make(chan struct{}, 1)
	ing := &fakeIngestor{produce: func(_ context.Context, ref string) (*domain.PublisherSnapshot, error) {
		select {
		case called <- struct{}{}:
		default:
		}
		return &domain.PublisherSnapshot{PackageRef: ref, LatestVersion: "ok"}, nil
	}}
	sch, err := publisher.NewScheduler(publisher.SchedulerConfig{
		Storage:         store,
		Ingestors:       map[string]publisher.Ingestor{"npm:": ing},
		RefreshInterval: time.Hour, // long enough that the test relies on the immediate-tick
		BatchSize:       10,
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- sch.Run(ctx) }()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not fire its immediate tick")
	}

	cancel()
	select {
	case err := <-done:
		require.True(t, errors.Is(err, context.Canceled),
			"want context.Canceled, got %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after ctx cancel")
	}
}
