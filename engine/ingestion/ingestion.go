// Package ingestion wraps a parser-produced domain.ParsedSBOM with the
// identity and deployment context that turn it into a domain.SBOM.
//
// Why this split exists: parsers (Go in-process, Rust over UDS) are
// pure — they own the lockfile-bytes-to-packages logic and nothing
// else. Stamping ID / GeneratedAt / ComponentRef / CommitSHA is an
// engine-side concern so parity tests can compare parser outputs
// byte-for-byte with zero shims. See ADR-0005.
package ingestion

import (
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Ingest lifts a ParsedSBOM into a full SBOM.
//
//   - `id` is a freshly-minted ULID (monotonic within the same
//     millisecond — ordered by creation time), which is why the incident
//     UI can list SBOMs without a secondary time index.
//   - `GeneratedAt` is UTC now on entry.
//   - `componentRef` and `commitSHA` flow through unchanged — callers
//     supply them from CLI flags, API requests, or the scaffolder task
//     that triggered the scan.
//
// Ingest is safe for concurrent use: `ulid.Make` is per-call and
// time.Now is inherently goroutine-safe.
func Ingest(parsed *domain.ParsedSBOM, componentRef, commitSHA string) *domain.SBOM {
	return &domain.SBOM{
		ID:           ulid.Make().String(),
		ComponentRef: componentRef,
		CommitSHA:    commitSHA,
		Ecosystem:    parsed.Ecosystem,
		GeneratedAt:  time.Now().UTC(),
		Packages:     parsed.Packages,
		SourceFormat: parsed.SourceFormat,
		SourceBytes:  parsed.SourceBytes,
	}
}
