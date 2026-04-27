// Package pypi parses Python lockfiles (requirements.txt, poetry.lock,
// uv.lock) into domain.ParsedSBOM values.
//
// Three formats, two underlying shapes:
//
//   - requirements.txt — line-oriented text. Each non-comment line is
//     `name[extras]==version` (other operators like >= / ~= are valid
//     pip syntax but not lockfile-strict; we accept only `==` and skip
//     anything else with a debug-level log).
//   - poetry.lock — TOML, root key `package = [{name, version, ...}]`
//     emitted as `[[package]]` blocks.
//   - uv.lock — also TOML with `[[package]]` blocks; the schema differs
//     slightly (uv has `source = { registry = … }` etc.) but we only
//     need name + version, so a single lenient TOML reader covers both.
//
// VCS dependencies (`git+https://…`), local-path requirements
// (`-e ./local-pkg`), and unpinned requirements (anything without
// `==`) are skipped silently — they have no version we can match an
// IoC against.
//
// PURL canonical form: `pkg:pypi/<name>@<version>`. Names are normalised
// to lowercase per PEP 503 so an IoC published against "Django" matches
// a lockfile entry written as "django".
//
// No Rust sidecar parity. Per ROADMAP, PyPI ships single-engine until
// the Wasm parity bridge lands in v0.5.0+; the parity_test pattern from
// gomod / cargo intentionally absent here.
package pypi

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

var ErrMalformedLockfile = errors.New("malformed lockfile")

// Format identifies which of the three pypi lockfile dialects the
// caller is feeding the parser. Set by the CLI from the filename
// (resolveEcosystem in cli/internal/commands/scan.go).
type Format string

const (
	FormatRequirements Format = "requirements" // requirements.txt
	FormatPoetry       Format = "poetry"       // poetry.lock
	FormatUV           Format = "uv"           // uv.lock
)

// Parser is stateless and safe for concurrent use.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

// Parse reads the lockfile content. format selects the parser.
// Errors wrap ErrMalformedLockfile.
func (p *Parser) Parse(ctx context.Context, content []byte, format Format) (*domain.ParsedSBOM, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var (
		pkgs   []domain.PackageVersion
		source string
		err    error
	)
	switch format {
	case FormatRequirements:
		pkgs, err = parseRequirements(content)
		source = "requirements-txt-v1"
	case FormatPoetry:
		pkgs, err = parseTOMLPackages(content)
		source = "poetry-lock-v2"
	case FormatUV:
		pkgs, err = parseTOMLPackages(content)
		source = "uv-lock-v1"
	default:
		return nil, fmt.Errorf("%w: unknown format %q", ErrMalformedLockfile, format)
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Version < pkgs[j].Version
	})
	return &domain.ParsedSBOM{
		Ecosystem:    "pypi",
		Packages:     pkgs,
		SourceFormat: source,
		SourceBytes:  int64(len(content)),
	}, nil
}

// parseRequirements consumes a requirements.txt body. We only accept
// `name==version` lines because anything looser (>=, ~=, no operator)
// is a constraint, not a pinned version, and IoC matching needs an
// exact version to compare against. Invalid lines are skipped, not
// fatal, so a partial requirements.txt still produces a usable SBOM.
func parseRequirements(content []byte) ([]domain.PackageVersion, error) {
	var pkgs []domain.PackageVersion
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seen := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comment.
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		// Skip pip directives (-e, -r, --hash=…) and VCS / local URLs.
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "git+") ||
			strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") ||
			strings.HasPrefix(line, "./") || strings.HasPrefix(line, "/") {
			continue
		}
		// `name==version` is the only pinned-version form pip emits in
		// a freeze; anything else (>=, ~=) is a range, not a lock.
		idx := strings.Index(line, "==")
		if idx <= 0 {
			continue
		}
		nameRaw := strings.TrimSpace(line[:idx])
		version := strings.TrimSpace(line[idx+2:])
		// Drop any trailing post-version markers ("; python_version >= ...").
		if semi := strings.Index(version, ";"); semi >= 0 {
			version = strings.TrimSpace(version[:semi])
		}
		// Strip extras: `package[extra1,extra2]` → `package`.
		if br := strings.Index(nameRaw, "["); br >= 0 {
			nameRaw = nameRaw[:br]
		}
		name := normalisePyPIName(nameRaw)
		if name == "" || version == "" {
			continue
		}
		key := name + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "pypi",
			Name:      name,
			Version:   version,
			PURL:      domain.CanonicalPURL("pypi", name, version),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: requirements scan: %v", ErrMalformedLockfile, err)
	}
	return pkgs, nil
}

// tomlLock is the lenient shape both poetry.lock and uv.lock honour at
// their root: a `package` array of objects each carrying at least
// `name` + `version`. Other fields (source, dependencies, hashes) are
// ignored — the matcher only needs the (name, version) tuple.
type tomlLock struct {
	Package []struct {
		Name    string `toml:"name"`
		Version string `toml:"version"`
		// Source is informational only; uv writes it as a table, poetry
		// often as a string. Unmarshalling into interface{} keeps both
		// shapes from blowing up the decoder.
		Source any `toml:"source"`
	} `toml:"package"`
}

func parseTOMLPackages(content []byte) ([]domain.PackageVersion, error) {
	var doc tomlLock
	if err := toml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("%w: toml decode: %v", ErrMalformedLockfile, err)
	}
	pkgs := make([]domain.PackageVersion, 0, len(doc.Package))
	seen := map[string]bool{}
	for _, p := range doc.Package {
		name := normalisePyPIName(strings.TrimSpace(p.Name))
		version := strings.TrimSpace(p.Version)
		if name == "" || version == "" {
			continue
		}
		key := name + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "pypi",
			Name:      name,
			Version:   version,
			PURL:      domain.CanonicalPURL("pypi", name, version),
		})
	}
	return pkgs, nil
}

// normalisePyPIName implements PEP 503's name canonicalisation: lowercase
// + collapse runs of `_`, `.`, `-` into a single `-`. So `Django`,
// `django`, `DJANGO`, and `dj-ango` all key the same way an IoC
// publisher would.
func normalisePyPIName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		if r == '_' || r == '.' || r == '-' {
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
			continue
		}
		b.WriteRune(r)
		prevDash = false
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}
