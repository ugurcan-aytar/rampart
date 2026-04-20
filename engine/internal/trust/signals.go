package trust

import (
	"context"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Detector inspects a PublisherProfile for a single anomaly class. A
// statistical Engine composes multiple Detectors and aggregates their output.
// Phase 3 plugs concrete Detectors — one per domain.SignalType — behind this
// interface; AlwaysTrust calls none.
type Detector interface {
	// Name identifies the detector for telemetry and explainability.
	Name() string

	// Detect returns a signal if the profile matches the detector's heuristic.
	// A nil signal with nil error means "no anomaly from this detector".
	Detect(ctx context.Context, profile domain.PublisherProfile) (*domain.PublisherSignal, error)
}
