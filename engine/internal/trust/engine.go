// Package trust houses the publisher-trust engine.
//
// Phase 1 (this sprint): the domain types, the Engine interface, and a default
// AlwaysTrust implementation that reports no signals. Phase 3 swaps in a
// statistical baseline wired from multiple Detector implementations — the
// interface in signals.go is the hook.
package trust

import (
	"context"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Engine evaluates whether a publisher's recent activity is anomalous.
//
// A return of (nil, nil) means "this engine found nothing"; it does NOT mean
// "the publisher is trusted" — absence of signal is not proof of safety.
type Engine interface {
	Evaluate(ctx context.Context, profile domain.PublisherProfile) ([]domain.PublisherSignal, error)
}
