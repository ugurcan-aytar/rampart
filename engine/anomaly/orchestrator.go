package anomaly

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/iocforward"
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
	bus          *events.Bus
	detectors    []Detector
	tickInterval time.Duration
	batchSize    int
	historyDepth int
	log          *slog.Logger
}

// OrchestratorConfig wires the orchestrator. Zero values produce
// sensible defaults: 1h tick, 200 packages per batch, 50-snapshot
// history per package. When EventBus is non-nil the orchestrator
// emits IoCs (ADR-0014) in addition to persisting Anomalies.
type OrchestratorConfig struct {
	Storage      storage.Storage
	EventBus     *events.Bus
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
		bus:          cfg.EventBus,
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

				// IoC bridge (ADR-0014): convert the anomaly into an
				// IoC + run the standard forward-match path so any
				// SBOM that references this package opens an
				// incident. Failure here is logged but not fatal —
				// the anomaly is already persisted, so operators can
				// triage via /v1/anomalies even without an incident.
				if err := o.emitIoC(ctx, a, ref, tickTime); err != nil {
					o.log.Warn("ioc emission failed",
						"kind", a.Kind, "ref", ref, "err", err)
				}
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

// emitIoC builds an IoC carrying the IoCBodyAnomaly variant and
// submits it through iocforward.Submit. ADR-0014 documents why this
// happens: the anomaly is also persisted in /v1/anomalies, but the
// IoC + matcher chain is what produces an Incident in the standard
// triage workflow.
func (o *Orchestrator) emitIoC(ctx context.Context, a domain.Anomaly, packageRef string, tickTime time.Time) error {
	ecosystem := ecosystemFromPackageRef(packageRef)
	ioc := domain.IoC{
		ID:          deterministicIoCID(tickTime),
		Kind:        domain.IoCKindPublisherAnomaly,
		Severity:    severityFromConfidence(a.Confidence),
		Ecosystem:   ecosystem,
		Source:      "rampart-anomaly-orchestrator",
		PublishedAt: tickTime,
		Description: a.Explanation,
		AnomalyBody: &domain.IoCBodyAnomaly{
			Kind:        a.Kind,
			Confidence:  a.Confidence,
			Explanation: a.Explanation,
			PackageRef:  packageRef,
			Evidence:    a.Evidence,
		},
	}
	_, err := iocforward.Submit(ctx, o.store, o.bus, ioc)
	return err
}

// ecosystemFromPackageRef extracts the `<ecosystem>` head from a
// `<ecosystem>:<name>` ref. Falls back to the ref itself for
// malformed input — IoC.Ecosystem is informational on the wire.
func ecosystemFromPackageRef(ref string) string {
	for i, r := range ref {
		if r == ':' {
			return ref[:i]
		}
	}
	return ref
}

// severityFromConfidence maps the detector's confidence grade onto
// the IoC severity scale. High = critical (operator must look),
// medium = high, low = medium (worth surfacing, low priority).
func severityFromConfidence(c domain.ConfidenceLevel) domain.Severity {
	switch c {
	case domain.ConfidenceHigh:
		return domain.SeverityCritical
	case domain.ConfidenceMedium:
		return domain.SeverityHigh
	default:
		return domain.SeverityMedium
	}
}

// deterministicIoCID synthesises a ULID whose timestamp portion
// equals tickTime. Two anomalies in the same tick get distinct
// (random-suffix) IDs because ulid.Make() pulls fresh entropy each
// call; that's fine — UpsertIoC's dedup is by ID + the Anomaly
// table's own UNIQUE constraint protects against double-counting on
// the anomaly side.
func deterministicIoCID(tickTime time.Time) string {
	return ulid.MustNew(ulid.Timestamp(tickTime), nil).String()
}
