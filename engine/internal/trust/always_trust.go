package trust

import (
	"context"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// AlwaysTrust is the Phase 1 default. It returns no signals for any profile —
// the engine exists so incident flows can call into it uniformly, even when
// the real statistical detectors haven't been built yet.
type AlwaysTrust struct{}

// Compile-time interface check.
var _ Engine = AlwaysTrust{}

func (AlwaysTrust) Evaluate(context.Context, domain.PublisherProfile) ([]domain.PublisherSignal, error) {
	return nil, nil
}
