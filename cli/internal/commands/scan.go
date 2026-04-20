package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ugurcan-aytar/rampart/cli/internal/output"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// Scan parses an npm lockfile and writes the SBOM in the requested format.
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

	sbom, err := npm.NewParser().Parse(ctx, content, npm.LockfileMeta{
		SourcePath:   path,
		ComponentRef: *componentRef,
		CommitSHA:    *commitSHA,
	})
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Convert the engine's returned SBOM into the CLI-local output shape
	// field-by-field. This keeps the engine's domain package internal to
	// the engine module — the CLI only speaks the local public shape.
	shape := &output.SBOM{
		ID:           sbom.ID,
		ComponentRef: sbom.ComponentRef,
		CommitSHA:    sbom.CommitSHA,
		Ecosystem:    sbom.Ecosystem,
		GeneratedAt:  sbom.GeneratedAt,
		SourceFormat: sbom.SourceFormat,
		SourceBytes:  sbom.SourceBytes,
		Packages:     make([]output.PackageVersion, len(sbom.Packages)),
	}
	for i, p := range sbom.Packages {
		shape.Packages[i] = output.PackageVersion{
			Ecosystem: p.Ecosystem,
			Name:      p.Name,
			Version:   p.Version,
			PURL:      p.PURL,
			Scope:     p.Scope,
			Integrity: p.Integrity,
		}
	}

	formatter, err := output.Get(*format)
	if err != nil {
		return err
	}
	return formatter.Write(stdout, shape)
}
