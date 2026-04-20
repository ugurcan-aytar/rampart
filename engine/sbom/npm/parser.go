// Package npm parses npm package-lock.json (lockfileVersion: 3) into
// domain.ParsedSBOM values. v1 / v2 lockfiles return
// ErrUnsupportedLockfileVersion. The parser is intentionally pure —
// callers wrap the result with engine/internal/ingestion.Ingest to
// attach ID / GeneratedAt / ComponentRef / CommitSHA. See ADR-0005.
package npm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

var (
	ErrMalformedLockfile          = errors.New("malformed lockfile")
	ErrUnsupportedLockfileVersion = errors.New("unsupported lockfile version")
	ErrEmptyLockfile              = errors.New("empty lockfile")
)

// Parser is stateless and safe for concurrent use.
type Parser struct {
	log *slog.Logger
}

func NewParser() *Parser {
	return &Parser{log: slog.Default()}
}

func NewParserWithLogger(log *slog.Logger) *Parser {
	if log == nil {
		log = slog.Default()
	}
	return &Parser{log: log}
}

// Parse reads a package-lock.json body and returns a pure ParsedSBOM.
//
// Errors wrap one of the exported sentinels (ErrMalformedLockfile,
// ErrUnsupportedLockfileVersion, ErrEmptyLockfile) with detail context,
// so callers should use errors.Is.
func (p *Parser) Parse(ctx context.Context, content []byte) (*domain.ParsedSBOM, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lf Lockfile
	if err := json.Unmarshal(content, &lf); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedLockfile, err)
	}
	if lf.LockfileVersion != 3 {
		return nil, fmt.Errorf("%w: got %d, expected 3", ErrUnsupportedLockfileVersion, lf.LockfileVersion)
	}
	if lf.Packages == nil {
		return nil, ErrEmptyLockfile
	}

	pkgs := make([]domain.PackageVersion, 0, len(lf.Packages))
	for path, entry := range lf.Packages {
		if shouldSkip(path, entry) {
			continue
		}
		if entry.Version == "" {
			p.log.Warn("npm package has no version; skipping", "path", path)
			continue
		}
		if entry.Integrity == "" {
			p.log.Warn("npm package has no integrity hash", "path", path)
		}

		name := extractName(path)
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "npm",
			Name:      name,
			Version:   entry.Version,
			PURL:      domain.CanonicalPURL("npm", name, entry.Version),
			Scope:     buildScope(entry),
			Integrity: entry.Integrity,
		})
	}

	// Deterministic ordering — the parity test in `parity_test.go`
	// compares Go and Rust outputs byte-for-byte, so both sides must
	// agree on (name, version) ordering.
	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Version < pkgs[j].Version
	})

	return &domain.ParsedSBOM{
		Ecosystem:    "npm",
		Packages:     pkgs,
		SourceFormat: "npm-package-lock-v3",
		SourceBytes:  int64(len(content)),
	}, nil
}

// shouldSkip filters out entries that don't represent an installed third-party package:
//   - "" (root manifest)
//   - "packages/..." workspace source entries (the workspace itself, not its deps)
//   - entries with link: true (symlinks to workspace packages — deduped)
func shouldSkip(path string, entry LockPackage) bool {
	if path == "" || entry.Link {
		return true
	}
	if !strings.Contains(path, "node_modules/") {
		return true
	}
	return false
}

// extractName maps a lockfile package path to the bare package name:
//
//	"node_modules/axios"                            → "axios"
//	"node_modules/@types/node"                      → "@types/node"
//	"node_modules/outer/node_modules/nested"        → "nested"
//	"node_modules/outer/node_modules/@scope/nested" → "@scope/nested"
func extractName(path string) string {
	const marker = "node_modules/"
	idx := strings.LastIndex(path, marker)
	if idx < 0 {
		return path
	}
	return path[idx+len(marker):]
}

// buildScope converts the dev / optional / peer boolean trio into the ordered
// slice rampart persists on domain.PackageVersion.Scope. Returns nil (not []
// with length 0) when the package has no scope markers, so SBOMs stay tidy.
func buildScope(e LockPackage) []string {
	scope := make([]string, 0, 3)
	if e.Dev {
		scope = append(scope, "dev")
	}
	if e.Optional {
		scope = append(scope, "optional")
	}
	if e.Peer {
		scope = append(scope, "peer")
	}
	if len(scope) == 0 {
		return nil
	}
	return scope
}
