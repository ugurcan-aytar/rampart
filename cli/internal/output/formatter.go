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

// SBOM is the CLI's public view of a parsed lockfile. In "parse-only"
// mode (no --component-ref / --commit-sha) the identity fields are
// zero-valued and `omitempty` keeps them out of the JSON output — that
// is what ADR-0005's "parser is pure" split looks like at the CLI
// surface.
type SBOM struct {
	ID           string           `json:"ID,omitempty"`
	ComponentRef string           `json:"ComponentRef,omitempty"`
	CommitSHA    string           `json:"CommitSHA,omitempty"`
	Ecosystem    string           `json:"Ecosystem"`
	GeneratedAt  *time.Time       `json:"GeneratedAt,omitempty"`
	SourceFormat string           `json:"SourceFormat"`
	SourceBytes  int64            `json:"SourceBytes"`
	Packages     []PackageVersion `json:"Packages"`
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
