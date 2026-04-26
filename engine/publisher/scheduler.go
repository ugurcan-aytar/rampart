package publisher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
)

// Scheduler is the cron tick that re-ingests stale publisher snapshots.
// It is OFF by default at the engine level (see config.PublisherEnabled);
// callers that wire one in explicitly opt in.
//
// Per-tick algorithm:
//  1. ListPackagesNeedingRefresh(now - RefreshInterval, BatchSize)
//  2. For each package_ref, dispatch to the matching ecosystem
//     ingestor by `<eco>:` prefix.
//  3. SavePublisherSnapshot. Errors that map to httpx sentinels
//     (NotFound, RateLimited, ServerFailure) log + skip — the next
//     tick will retry. Other errors log at ERROR.
//
// Concurrency: one inflight tick at a time. Each tick is fully
// sequential — the ingestors are already rate-limited per-upstream
// and parallelism here would just compress the request burst into
// a single window.
type Scheduler struct {
	store           storage.Storage
	ingestors       map[string]Ingestor // keyed by `<eco>:` prefix (with trailing colon)
	refreshInterval time.Duration
	batchSize       int
	log             *slog.Logger
	now             func() time.Time // injected for tests; defaults to time.Now
}

// SchedulerConfig wires the scheduler. RefreshInterval defaults to 1h
// when zero; BatchSize defaults to 50.
type SchedulerConfig struct {
	Storage         storage.Storage
	Ingestors       map[string]Ingestor
	RefreshInterval time.Duration
	BatchSize       int
	Logger          *slog.Logger
}

// NewScheduler validates the config and returns a runnable scheduler.
func NewScheduler(cfg SchedulerConfig) (*Scheduler, error) {
	if cfg.Storage == nil {
		return nil, errors.New("publisher scheduler: Storage is required")
	}
	if len(cfg.Ingestors) == 0 {
		return nil, errors.New("publisher scheduler: at least one ingestor required")
	}
	for prefix := range cfg.Ingestors {
		if !strings.HasSuffix(prefix, ":") {
			return nil, fmt.Errorf("publisher scheduler: ingestor key %q must end with ':'", prefix)
		}
	}
	interval := cfg.RefreshInterval
	if interval <= 0 {
		interval = time.Hour
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 50
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		store:           cfg.Storage,
		ingestors:       cfg.Ingestors,
		refreshInterval: interval,
		batchSize:       batch,
		log:             log.With("component", "publisher.scheduler"),
		now:             func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run blocks until ctx is cancelled. Fires Tick once immediately so a
// freshly-started engine populates state without waiting an interval.
func (s *Scheduler) Run(ctx context.Context) error {
	s.log.Info("scheduler running",
		"refresh_interval", s.refreshInterval,
		"batch_size", s.batchSize,
		"ecosystems", ingestorKeys(s.ingestors))

	if err := s.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		s.log.Warn("first tick error (continuing)", "err", err)
	}

	t := time.NewTicker(s.refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopped", "reason", ctx.Err())
			return ctx.Err()
		case <-t.C:
			if err := s.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.log.Warn("tick error (continuing)", "err", err)
			}
		}
	}
}

// Tick runs a single refresh cycle. Exported for tests; Run loops it.
func (s *Scheduler) Tick(ctx context.Context) error {
	cutoff := s.now().Add(-s.refreshInterval)
	refs, err := s.store.ListPackagesNeedingRefresh(ctx, cutoff, s.batchSize)
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}
	if len(refs) == 0 {
		s.log.Debug("tick: no packages need refresh")
		return nil
	}
	s.log.Info("tick: refreshing packages", "count", len(refs))

	var ok, missing, throttled, failed int
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		ing := s.dispatch(ref)
		if ing == nil {
			s.log.Warn("no ingestor for package_ref; skipping", "ref", ref)
			failed++
			continue
		}
		snap, err := ing.Ingest(ctx, ref)
		if err != nil {
			switch {
			case errors.Is(err, httpx.ErrNotFound):
				s.log.Info("ingest: package not found upstream; will retry next tick", "ref", ref)
				missing++
			case errors.Is(err, httpx.ErrRateLimited):
				s.log.Warn("ingest: rate-limited", "ref", ref, "err", err)
				throttled++
			case errors.Is(err, httpx.ErrServerFailure):
				s.log.Warn("ingest: upstream 5xx (gave up)", "ref", ref, "err", err)
				failed++
			default:
				s.log.Error("ingest: unexpected error", "ref", ref, "err", err)
				failed++
			}
			continue
		}
		if err := s.store.SavePublisherSnapshot(ctx, *snap); err != nil {
			s.log.Error("save snapshot failed", "ref", ref, "err", err)
			failed++
			continue
		}
		ok++
	}
	s.log.Info("tick complete",
		"refreshed", ok, "missing", missing, "throttled", throttled, "failed", failed)
	return nil
}

// dispatch picks the ingestor whose `<eco>:` prefix matches packageRef.
// Returns nil for unknown prefixes — caller logs + skips.
func (s *Scheduler) dispatch(packageRef string) Ingestor {
	for prefix, ing := range s.ingestors {
		if strings.HasPrefix(packageRef, prefix) {
			return ing
		}
	}
	return nil
}

func ingestorKeys(m map[string]Ingestor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
