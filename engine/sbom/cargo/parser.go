// Package cargo parses Cargo.lock (TOML) into domain.ParsedSBOM
// values. The Rust sidecar mirror lives at
// `native/crates/rampart-native/src/parsers/cargo.rs`; the parity
// contract is enforced by `parity_test.go`.
//
// Workspace-local members (no `source` field) are skipped — they are
// the project itself, not pulled deps. Git-sourced packages get
// `Scope = ["git"]`; registry sources keep `Scope = nil`. Unknown
// source schemes are tagged `["source:<verbatim>"]` so an operator can
// audit them.
package cargo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

var (
	ErrMalformedLockfile = errors.New("malformed lockfile")
	ErrEmptyLockfile     = errors.New("empty lockfile")
)

const (
	registryPrefix = "registry+"
	gitPrefix      = "git+"
)

// Parser is stateless and safe for concurrent use.
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// Parse reads a Cargo.lock body and returns a pure ParsedSBOM.
//
// Errors wrap ErrMalformedLockfile or ErrEmptyLockfile.
func (p *Parser) Parse(ctx context.Context, content []byte) (*domain.ParsedSBOM, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lf Lockfile
	if _, err := toml.Decode(string(content), &lf); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedLockfile, err)
	}
	if len(lf.Packages) == 0 {
		return nil, ErrEmptyLockfile
	}

	pkgs := make([]domain.PackageVersion, 0, len(lf.Packages))
	for _, p := range lf.Packages {
		if p.Source == "" {
			// Workspace-local member; skip.
			continue
		}
		var scope []string
		switch {
		case strings.HasPrefix(p.Source, gitPrefix):
			scope = []string{"git"}
		case strings.HasPrefix(p.Source, registryPrefix):
			scope = nil
		default:
			scope = []string{"source:" + p.Source}
		}
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "cargo",
			Name:      p.Name,
			Version:   p.Version,
			PURL:      domain.CanonicalPURL("cargo", p.Name, p.Version),
			Scope:     scope,
			Integrity: p.Checksum,
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Version < pkgs[j].Version
	})

	return &domain.ParsedSBOM{
		Ecosystem:    "cargo",
		Packages:     pkgs,
		SourceFormat: "cargo-lock-v3",
		SourceBytes:  int64(len(content)),
	}, nil
}
