package domain

import "time"

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
