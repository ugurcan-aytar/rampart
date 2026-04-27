// Package anomaly hosts the publisher-anomaly detectors that turn
// PublisherSnapshot history (Theme F1) into Anomaly rows (Theme F2).
//
// Each ecosystem-agnostic detector implements [Detector]; the
// orchestrator in this package walks every package whose snapshot
// history changed since the last detection tick and runs every
// detector against the package's history.
//
// Detector contract:
//   - Detectors are PURE — given the same history they must produce
//     the same anomalies. No upstream calls, no clock reads.
//     `time.Now()` is allowed only via the injected `now` field on
//     each detector, so tests can drive deterministic timestamps.
//   - Detectors NEVER persist; the orchestrator persists the returned
//     slice via storage.SaveAnomaly (which is itself idempotent).
//   - Detectors return an empty slice when nothing fires; nil is
//     also acceptable. Errors should be reserved for genuinely
//     bad input (malformed semver, unparseable evidence) — a "no
//     anomaly today" outcome is not an error.
package anomaly

import (
	"context"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Detector evaluates a package's snapshot history and returns any
// anomalies it detects.
type Detector interface {
	// Kind names the AnomalyKind this detector produces. The
	// orchestrator uses this for logging + per-detector metrics.
	Kind() domain.AnomalyKind

	// Detect runs the detector over a single package's history.
	// History is expected newest-first (matches storage.GetPublisherHistory).
	Detect(ctx context.Context, packageRef string, history []domain.PublisherSnapshot) ([]domain.Anomaly, error)
}
