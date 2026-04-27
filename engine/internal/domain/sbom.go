package domain

import "time"

// ParsedSBOM is the pure result of lockfile parsing — no identity, no
// timestamp, no deployment context. Both the Go parser at
// `engine/sbom/npm` and the Rust sidecar at `native/` produce this
// shape. Turning a ParsedSBOM into a full [SBOM] (with ID, GeneratedAt,
// ComponentRef, CommitSHA) is the caller's job; see
// `engine/internal/ingestion.Ingest`. ADR-0005 records the split.
type ParsedSBOM struct {
	Ecosystem    string
	Packages     []PackageVersion
	SourceFormat string
	SourceBytes  int64
}

// SBOM is a historical, point-in-time dependency snapshot for a Component.
// (ComponentRef, CommitSHA) is unique — new scans produce new SBOMs, never
// mutate old ones.
type SBOM struct {
	ID           string
	ComponentRef string
	CommitSHA    string
	Ecosystem    string
	GeneratedAt  time.Time
	Packages     []PackageVersion
	SourceFormat string
	SourceBytes  int64
}

// SBOMPackageRef pairs a component reference with one of its SBOM
// packages. It is the row shape returned by Storage.ListSBOMPackages —
// the bulk-lookup hot path that replaced the matcher's per-component
// ListSBOMsByComponent loop. The same ComponentRef can appear more
// than once if multiple historical SBOMs (re-scans) carry the same
// package, or if a single SBOM lists the same name at multiple
// versions; callers are expected to dedupe on ComponentRef once
// matcher.Evaluate has confirmed the version predicate.
type SBOMPackageRef struct {
	ComponentRef string
	Package      PackageVersion
}
