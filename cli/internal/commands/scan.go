package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ugurcan-aytar/rampart/cli/internal/output"
	"github.com/ugurcan-aytar/rampart/engine/ingestion"
	"github.com/ugurcan-aytar/rampart/engine/sbom/cargo"
	"github.com/ugurcan-aytar/rampart/engine/sbom/gomod"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// Scan parses a lockfile and writes the result in the requested format.
//
// Ecosystem is auto-detected from the filename when --ecosystem is not
// supplied:
//
//   - package-lock.json → npm
//   - go.sum            → gomod (sibling go.mod is read if present)
//   - Cargo.lock        → cargo
//
// With neither --component-ref nor --commit-sha, Scan emits the pure
// ParsedSBOM (no ID, no GeneratedAt) — the bytes coming straight out of
// the parser. When at least one identity flag is supplied, Scan wraps
// the result through the engine's ingestion layer and emits a full SBOM.
//
// Supported formats: text (default), json, sarif.
func Scan(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "text", "output format: text | json | sarif")
	componentRef := fs.String("component-ref", "", "component reference (e.g. kind:Component/default/web-app)")
	commitSHA := fs.String("commit-sha", "", "commit sha the lockfile was taken at")
	ecosystem := fs.String("ecosystem", "", "force ecosystem: npm | gomod | cargo (default: auto-detect from filename)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rampart scan [--format text|json|sarif] [--ecosystem npm|gomod|cargo] [--component-ref ref] [--commit-sha sha] <lockfile>")
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

	eco, err := resolveEcosystem(*ecosystem, path)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	// The cli package can't import engine/internal/domain, so each branch
	// converts the parser-specific result into the output shape inline
	// rather than via a shared helper that would have to name the type.
	var (
		shape       *output.SBOM
		id          string
		genAt       *time.Time
		fullCompRef string
		fullCommit  string
	)
	switch eco {
	case "npm":
		parsed, err := npm.NewParser().Parse(ctx, content)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		shape = &output.SBOM{
			Ecosystem:    parsed.Ecosystem,
			SourceFormat: parsed.SourceFormat,
			SourceBytes:  parsed.SourceBytes,
			Packages:     make([]output.PackageVersion, len(parsed.Packages)),
		}
		for i, p := range parsed.Packages {
			shape.Packages[i] = output.PackageVersion{Ecosystem: p.Ecosystem, Name: p.Name, Version: p.Version, PURL: p.PURL, Scope: p.Scope, Integrity: p.Integrity}
		}
		if *componentRef != "" || *commitSHA != "" {
			full := ingestion.Ingest(parsed, *componentRef, *commitSHA)
			id, fullCompRef, fullCommit = full.ID, full.ComponentRef, full.CommitSHA
			gen := full.GeneratedAt
			genAt = &gen
		}
	case "gomod":
		var gomodContent []byte
		if filepath.Base(path) == "go.sum" {
			if b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "go.mod")); err == nil {
				gomodContent = b
			}
		}
		parsed, err := gomod.NewParser().Parse(ctx, content, gomodContent)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		shape = &output.SBOM{
			Ecosystem:    parsed.Ecosystem,
			SourceFormat: parsed.SourceFormat,
			SourceBytes:  parsed.SourceBytes,
			Packages:     make([]output.PackageVersion, len(parsed.Packages)),
		}
		for i, p := range parsed.Packages {
			shape.Packages[i] = output.PackageVersion{Ecosystem: p.Ecosystem, Name: p.Name, Version: p.Version, PURL: p.PURL, Scope: p.Scope, Integrity: p.Integrity}
		}
		if *componentRef != "" || *commitSHA != "" {
			full := ingestion.Ingest(parsed, *componentRef, *commitSHA)
			id, fullCompRef, fullCommit = full.ID, full.ComponentRef, full.CommitSHA
			gen := full.GeneratedAt
			genAt = &gen
		}
	case "cargo":
		parsed, err := cargo.NewParser().Parse(ctx, content)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		shape = &output.SBOM{
			Ecosystem:    parsed.Ecosystem,
			SourceFormat: parsed.SourceFormat,
			SourceBytes:  parsed.SourceBytes,
			Packages:     make([]output.PackageVersion, len(parsed.Packages)),
		}
		for i, p := range parsed.Packages {
			shape.Packages[i] = output.PackageVersion{Ecosystem: p.Ecosystem, Name: p.Name, Version: p.Version, PURL: p.PURL, Scope: p.Scope, Integrity: p.Integrity}
		}
		if *componentRef != "" || *commitSHA != "" {
			full := ingestion.Ingest(parsed, *componentRef, *commitSHA)
			id, fullCompRef, fullCommit = full.ID, full.ComponentRef, full.CommitSHA
			gen := full.GeneratedAt
			genAt = &gen
		}
	default:
		return fmt.Errorf("scan: unknown ecosystem %q", eco)
	}

	if id != "" {
		shape.ID, shape.ComponentRef, shape.CommitSHA, shape.GeneratedAt = id, fullCompRef, fullCommit, genAt
	}

	formatter, err := output.Get(*format)
	if err != nil {
		return err
	}
	return formatter.Write(stdout, shape)
}

// resolveEcosystem honours an explicit --ecosystem flag, otherwise
// auto-detects from filename. Unknown filenames default to npm so
// existing scripts that pass arbitrarily-named npm lockfiles keep
// working — the historical CLI was npm-only.
func resolveEcosystem(explicit, path string) (string, error) {
	if explicit != "" {
		switch explicit {
		case "npm", "gomod", "cargo":
			return explicit, nil
		default:
			return "", fmt.Errorf("scan: unknown --ecosystem %q (want: npm | gomod | cargo)", explicit)
		}
	}
	switch filepath.Base(path) {
	case "go.sum":
		return "gomod", nil
	case "Cargo.lock":
		return "cargo", nil
	default:
		return "npm", nil
	}
}
