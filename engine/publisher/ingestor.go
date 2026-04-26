// Package publisher hosts the upstream-API ingestors that produce
// PublisherSnapshot rows in storage. Each ecosystem-specific ingestor
// (npm, github, …) implements [Ingestor]; the cron scheduler in
// engine/publisher/scheduler.go dispatches refresh ticks to whichever
// ingestor matches the package_ref's `<ecosystem>:` prefix.
//
// The split between this package and the ecosystem-specific
// sub-packages (engine/publisher/npm, engine/publisher/github) keeps
// each upstream's quirks (auth, rate-limit headers, response shape)
// isolated; the orchestration layer here only sees the
// `Ingest(ctx, packageRef) → PublisherSnapshot` contract.
package publisher

import (
	"context"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Ingestor fetches a single package's publisher metadata from an
// upstream registry. Implementations are responsible for their own
// rate limiting + retry policy; the scheduler treats them as opaque.
type Ingestor interface {
	// Ingest produces a PublisherSnapshot for packageRef. The returned
	// snapshot has SnapshotAt left to the storage layer (so all
	// snapshots in a single tick share a clock); ID is also assigned
	// downstream. Errors are returned to the scheduler which decides
	// whether to log + skip (rate-limited / not-found) or retry.
	Ingest(ctx context.Context, packageRef string) (*domain.PublisherSnapshot, error)
}
