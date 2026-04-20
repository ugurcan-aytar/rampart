// Package output houses the CLI's rendering layer. Each Formatter takes
// a *SBOM (CLI's own public shape) and writes its rendering to an io.Writer.
//
// SBOM / PackageVersion here are intentionally decoupled from the engine's
// internal domain types: the CLI is a separate module and must not leak
// engine internals. Fields are copied in cli/internal/commands/scan.go.
package output

import (
	"fmt"
	"io"
	"time"
)

// SBOM is the CLI's public view of a parsed lockfile.
type SBOM struct {
	ID           string
	ComponentRef string
	CommitSHA    string
	Ecosystem    string
	GeneratedAt  time.Time
	SourceFormat string
	SourceBytes  int64
	Packages     []PackageVersion
}

// PackageVersion mirrors engine/sbom.Package — kept local to the CLI so
// the engine's domain package stays internal.
type PackageVersion struct {
	Ecosystem string
	Name      string
	Version   string
	PURL      string
	Scope     []string
	Integrity string
}

// Formatter renders an SBOM to text, JSON, or SARIF.
type Formatter interface {
	Write(w io.Writer, sbom *SBOM) error
}

// Get resolves format name to a Formatter. "" falls through to "text".
func Get(format string) (Formatter, error) {
	switch format {
	case "", "text":
		return Text{}, nil
	case "json":
		return JSON{}, nil
	case "sarif":
		return SARIF{}, nil
	}
	return nil, fmt.Errorf("unknown format %q (text | json | sarif)", format)
}
