package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ugurcan-aytar/rampart/cli/internal/output"
	"github.com/ugurcan-aytar/rampart/engine/ingestion"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// Scan parses an npm lockfile and writes the result in the requested format.
//
// With neither --component-ref nor --commit-sha, Scan emits the pure
// ParsedSBOM (no ID, no GeneratedAt) — the bytes coming straight out of
// the parser. When at least one identity flag is supplied, Scan wraps
// the result through the engine's ingestion layer and emits a full
// SBOM. Mirrors `engine parse-sbom` so the two entry points behave the
// same for the same inputs.
//
// Supported formats: text (default), json, sarif.
func Scan(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "text", "output format: text | json | sarif")
	componentRef := fs.String("component-ref", "", "component reference (e.g. kind:Component/default/web-app)")
	commitSHA := fs.String("commit-sha", "", "commit sha the lockfile was taken at")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rampart scan [--format text|json|sarif] [--component-ref ref] [--commit-sha sha] <lockfile>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("scan: missing lockfile path")
	}
	path := fs.Arg(0)

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	parsed, err := npm.NewParser().Parse(ctx, content)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	shape := &output.SBOM{
		Ecosystem:    parsed.Ecosystem,
		SourceFormat: parsed.SourceFormat,
		SourceBytes:  parsed.SourceBytes,
		Packages:     make([]output.PackageVersion, len(parsed.Packages)),
	}
	for i, p := range parsed.Packages {
		shape.Packages[i] = output.PackageVersion{
			Ecosystem: p.Ecosystem,
			Name:      p.Name,
			Version:   p.Version,
			PURL:      p.PURL,
			Scope:     p.Scope,
			Integrity: p.Integrity,
		}
	}

	if *componentRef != "" || *commitSHA != "" {
		full := ingestion.Ingest(parsed, *componentRef, *commitSHA)
		gen := full.GeneratedAt
		shape.ID = full.ID
		shape.ComponentRef = full.ComponentRef
		shape.CommitSHA = full.CommitSHA
		shape.GeneratedAt = &gen
	}

	formatter, err := output.Get(*format)
	if err != nil {
		return err
	}
	return formatter.Write(stdout, shape)
}
