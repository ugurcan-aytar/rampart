package anomaly

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Orchestrator runs the configured detectors over recently-active
// packages on a cron tick. OFF by default at the engine level (see
// config.AnomalyEnabled); callers wire one in explicitly to opt in.
//
// Per-tick algorithm:
//  1. Pick the packages whose snapshot history is "fresh enough" —
//     `ListPackagesNeedingRefresh(now - StaleWindow, BatchSize)` is
//     misleadingly named here: we use it as a recently-touched index
//     by inverting the threshold (now + future ≈ "all packages with
//     ANY snapshot"). For F2 we re-evaluate every package on every
//     tick — SaveAnomaly is idempotent so re-runs are cheap.
//  2. For each package, fetch its history (capped at HistoryDepth)
//     and run each detector.
//  3. Save every returned Anomaly. Dedup is enforced storage-side
//     via UNIQUE (kind, package_ref, detected_at).
//
// Time discipline: the entire tick uses a single frozen timestamp.
// Detectors may have their own injected clock for windowing (e.g.
// "is the latest publish ≤ 7d ago?") but the SAVED Anomaly.DetectedAt
// is always the tick time. This makes the dedup behaviour
// deterministic — a re-run of the same tick at the same wall time is
// a no-op.
type Orchestrator struct {
	store        storage.Storage
	detectors    []Detector
	tickInterval time.Duration
	batchSize    int
	historyDepth int
	log          *slog.Logger
}

// OrchestratorConfig wires the orchestrator. Zero values produce
// sensible defaults: 1h tick, 200 packages per batch, 50-snapshot
// history per package.
type OrchestratorConfig struct {
	Storage      storage.Storage
	Detectors    []Detector
	TickInterval time.Duration
	BatchSize    int
	HistoryDepth int
	Logger       *slog.Logger
}

// NewOrchestrator validates the config and returns a runnable orchestrator.
func NewOrchestrator(cfg OrchestratorConfig) (*Orchestrator, error) {
	if cfg.Storage == nil {
		return nil, errors.New("anomaly orchestrator: Storage is required")
	}
	if len(cfg.Detectors) == 0 {
		return nil, errors.New("anomaly orchestrator: at least one detector required")
	}
	tick := cfg.TickInterval
	if tick <= 0 {
		tick = time.Hour
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 200
	}
	depth := cfg.HistoryDepth
	if depth <= 0 {
		depth = 50
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Orchestrator{
		store:        cfg.Storage,
		detectors:    cfg.Detectors,
		tickInterval: tick,
		batchSize:    batch,
		historyDepth: depth,
		log:          log.With("component", "anomaly.orchestrator"),
	}, nil
}

// Run blocks until ctx is cancelled. Fires Tick once immediately so a
// freshly-started engine evaluates current state without waiting an
// interval.
func (o *Orchestrator) Run(ctx context.Context) error {
	o.log.Info("orchestrator running",
		"tick_interval", o.tickInterval,
		"batch_size", o.batchSize,
		"history_depth", o.historyDepth,
		"detector_count", len(o.detectors))

	if err := o.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		o.log.Warn("first tick error (continuing)", "err", err)
	}

	t := time.NewTicker(o.tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			o.log.Info("orchestrator stopped", "reason", ctx.Err())
			return ctx.Err()
		case <-t.C:
			if err := o.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				o.log.Warn("tick error (continuing)", "err", err)
			}
		}
	}
}

// Tick runs a single detection cycle. Exported for tests; Run loops it.
func (o *Orchestrator) Tick(ctx context.Context) error {
	tickTime := time.Now().UTC()
	// Pick recently-touched packages. We use a "future" threshold so
	// every package with any snapshot returns; the storage call's
	// real purpose at F2 is "give me an indexed list of package_refs"
	// and the SaveAnomaly idempotency takes care of duplicates.
	refs, err := o.store.ListPackagesNeedingRefresh(ctx, tickTime.Add(time.Hour), o.batchSize)
	if err != nil {
		return fmt.Errorf("anomaly tick: list packages: %w", err)
	}
	if len(refs) == 0 {
		o.log.Debug("tick: no packages to evaluate")
		return nil
	}
	o.log.Info("tick: evaluating packages", "count", len(refs))

	var raised, skipped, failed int
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		history, err := o.store.GetPublisherHistory(ctx, ref, o.historyDepth)
		if err != nil {
			o.log.Error("get publisher history failed", "ref", ref, "err", err)
			failed++
			continue
		}
		if len(history) < 2 {
			skipped++
			continue
		}
		for _, det := range o.detectors {
			anomalies, err := det.Detect(ctx, ref, history)
			if err != nil {
				o.log.Error("detector failed",
					"detector", det.Kind(), "ref", ref, "err", err)
				failed++
				continue
			}
			for _, a := range anomalies {
				// Freeze DetectedAt to the tick time so re-runs are
				// idempotent against the storage UNIQUE constraint.
				a.DetectedAt = tickTime
				if _, err := o.store.SaveAnomaly(ctx, a); err != nil {
					o.log.Error("save anomaly failed",
						"detector", det.Kind(), "ref", ref, "err", err)
					failed++
					continue
				}
				raised++
				o.log.Info("anomaly raised",
					"kind", a.Kind, "ref", ref, "confidence", a.Confidence)
			}
		}
	}
	o.log.Info("tick complete",
		"packages_evaluated", len(refs),
		"raised", raised,
		"skipped", skipped,
		"failed", failed)
	return nil
}
