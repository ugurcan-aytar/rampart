// Package gomod parses Go module lockfiles (go.sum + go.mod) into
// domain.ParsedSBOM values. The Rust sidecar mirror lives at
// `native/crates/rampart-native/src/parsers/gomod.rs`; the parity
// contract is enforced by `parity_test.go`.
//
// Algorithm:
//  1. Parse go.mod for `replace OLD [VER] => NEW [VER]` directives. Local
//     file-system replaces (`=> ./local`) drop the source entry from the
//     output. Remote replaces substitute the right-hand-side path + version.
//  2. Parse go.sum line-by-line. Each line:
//     `<module-path> <version>[/go.mod] h1:<hash>=`
//     The primary line (no `/go.mod` suffix) carries the module zip hash;
//     the `/go.mod` line hashes go.mod itself. Prefer the primary; modules
//     that only have the `/go.mod` line still get an entry.
//  3. Apply replaces, sort by (name, version), emit.
//
// Pseudo-versions (`v0.0.0-YYYYMMDDHHMMSS-abcdef123456`) are passed
// through unchanged — they are still valid module versions.
package gomod

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

var (
	ErrMalformedLockfile = errors.New("malformed lockfile")
)

// Parser is stateless and safe for concurrent use.
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// Parse reads go.sum (authoritative version + hash list) and go.mod
// (module identity + replace directives) and returns a pure ParsedSBOM.
// gomodContent may be nil/empty (e.g. tarball that ships only go.sum) —
// in that case no replace directives apply.
//
// Errors wrap ErrMalformedLockfile.
func (p *Parser) Parse(ctx context.Context, gosumContent, gomodContent []byte) (*domain.ParsedSBOM, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	replaces, err := parseGomodReplaces(gomodContent)
	if err != nil {
		return nil, err
	}
	entries, err := parseGosum(gosumContent)
	if err != nil {
		return nil, err
	}

	// Dedup (module, version) pairs. The h1: line wins over the /go.mod
	// line; if only the /go.mod line is present (rare but legal), use it.
	type key struct {
		module, version string
	}
	hashes := make(map[key]string)
	primary := make(map[key]bool)
	for _, e := range entries {
		k := key{module: e.Module, version: e.Version}
		if primary[k] {
			continue
		}
		hashes[k] = e.Hash
		primary[k] = e.IsPrimary
	}

	pkgs := make([]domain.PackageVersion, 0, len(hashes))
	// Iterate in deterministic order (key sort) so the intermediate
	// step doesn't depend on map randomisation; final sort below
	// guarantees output ordering anyway.
	keys := make([]key, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].module != keys[j].module {
			return keys[i].module < keys[j].module
		}
		return keys[i].version < keys[j].version
	})
	for _, k := range keys {
		name := k.module
		ver := k.version
		if target, ok := replaces[k.module]; ok {
			if target.NewVersion == "" {
				continue // local replace drops the source
			}
			name = target.NewPath
			ver = target.NewVersion
		}
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "gomod",
			Name:      name,
			Version:   ver,
			PURL:      domain.CanonicalPURL("gomod", name, ver),
			Integrity: hashes[k],
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Version < pkgs[j].Version
	})

	return &domain.ParsedSBOM{
		Ecosystem:    "gomod",
		Packages:     pkgs,
		SourceFormat: "go-sum-v1",
		SourceBytes:  int64(len(gosumContent)),
	}, nil
}

func parseGosum(content []byte) ([]sumEntry, error) {
	var out []sumEntry
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%w: go.sum line %d: expected 3 fields, got %d",
				ErrMalformedLockfile, lineno, len(parts))
		}
		module, rawVersion, hash := parts[0], parts[1], parts[2]
		if !strings.HasPrefix(hash, "h1:") {
			return nil, fmt.Errorf("%w: go.sum line %d: hash field must use the h1 prefix",
				ErrMalformedLockfile, lineno)
		}
		version := rawVersion
		isPrimary := true
		if strings.HasSuffix(rawVersion, "/go.mod") {
			version = strings.TrimSuffix(rawVersion, "/go.mod")
			isPrimary = false
		}
		out = append(out, sumEntry{
			Module:    module,
			Version:   version,
			Hash:      hash,
			IsPrimary: isPrimary,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: go.sum scan: %v", ErrMalformedLockfile, err)
	}
	return out, nil
}

func parseGomodReplaces(content []byte) (map[string]replaceTarget, error) {
	out := make(map[string]replaceTarget)
	if len(content) == 0 {
		return out, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inBlock := false
	for scanner.Scan() {
		line := strings.TrimSpace(stripLineComment(scanner.Text()))
		if line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			if src, tgt, ok := parseReplaceDirective(line); ok {
				out[src] = tgt
			}
			continue
		}
		if line == "replace (" {
			inBlock = true
			continue
		}
		if rest, ok := strings.CutPrefix(line, "replace "); ok {
			if src, tgt, ok := parseReplaceDirective(strings.TrimSpace(rest)); ok {
				out[src] = tgt
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: go.mod scan: %v", ErrMalformedLockfile, err)
	}
	return out, nil
}

func stripLineComment(s string) string {
	if i := strings.Index(s, "//"); i >= 0 {
		return s[:i]
	}
	return s
}

// parseReplaceDirective handles `OLD [OLDVER] => NEW [NEWVER]`.
// Malformed lines are silently skipped (they would already fail
// `go mod tidy`); the parser treats go.mod as best-effort metadata.
func parseReplaceDirective(line string) (string, replaceTarget, bool) {
	parts := strings.SplitN(line, "=>", 2)
	if len(parts) != 2 {
		return "", replaceTarget{}, false
	}
	lhs := strings.Fields(parts[0])
	rhs := strings.Fields(parts[1])
	if len(lhs) == 0 || len(rhs) == 0 {
		return "", replaceTarget{}, false
	}
	src := lhs[0]
	newPath := rhs[0]
	// Local replace: starts with . or /, OR has no version field.
	isLocal := strings.HasPrefix(newPath, ".") || strings.HasPrefix(newPath, "/") || len(rhs) == 1
	tgt := replaceTarget{NewPath: newPath}
	if !isLocal && len(rhs) >= 2 {
		tgt.NewVersion = rhs[1]
	}
	return src, tgt, true
}
