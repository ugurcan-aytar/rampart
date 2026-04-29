// Package trust houses the publisher-trust engine.
//
// Initial implementation: domain types, the Engine interface, and a
// default AlwaysTrust implementation that reports no signals. A
// statistical baseline wired from multiple Detector implementations
// can swap in via the interface in signals.go when scoped (no
// specific theme yet).
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
