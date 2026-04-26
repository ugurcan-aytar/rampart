package domain

import (
	"encoding/json"
	"time"
)

// PublisherSnapshot is a point-in-time view of a single package's
// publisher metadata (maintainers, latest version, publish method,
// source repo). The snapshot history of a package is what Theme F2's
// anomaly detector diffs to raise PublisherSignal events.
//
// PackageRef is `<ecosystem>:<name>` — e.g. `npm:axios`,
// `gomod:github.com/spf13/cobra`, `cargo:tokio`. The colon prefix is
// the same convention `engine/sbom/<eco>` parsers emit on
// `PackageVersion.Ecosystem`, so a single snapshot row maps cleanly
// back to any SBOM entry.
type PublisherSnapshot struct {
	ID         int64
	PackageRef string
	SnapshotAt time.Time

	Maintainers              []Maintainer
	LatestVersion            string
	LatestVersionPublishedAt *time.Time
	PublishMethod            string // "oidc-trusted-publisher" | "token" | "unknown"
	SourceRepoURL            *string

	// RawData is the raw upstream API payload (npm registry response /
	// GitHub release JSON), kept verbatim for two reasons: (a) lets
	// Theme F2 detectors evolve the parsed fields without re-fetching,
	// (b) auditable trail of what the engine actually saw at SnapshotAt.
	RawData json.RawMessage
}

// Maintainer is a single (email, name, username) tuple from a
// registry's maintainers list. All three fields are optional — npm
// guarantees email + name; some legacy registry responses only carry
// username.
type Maintainer struct {
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
}
