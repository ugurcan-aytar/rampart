// Package maven parses JVM lockfiles (Maven pom.xml + Gradle's
// gradle.lockfile) into domain.ParsedSBOM values.
//
// Two formats:
//
//   - pom.xml — Maven's project file. Reads `<project><dependencies>
//     <dependency><groupId>+<artifactId>+<version>` triples.
//     `<scope>test</scope>` entries are kept (carried in
//     PackageVersion.Scope) so a downstream filter can drop them; we
//     do not drop them at parse time. Property substitution
//     (`${spring.version}`) is resolved against `<project><properties>`;
//     unresolved placeholders are kept verbatim with their `${…}`
//     intact, which makes the residue visible in SBOM output rather
//     than silently empty.
//   - gradle.lockfile — Gradle's deterministic lockfile, line format
//     `groupId:artifactId:version=tag1,tag2,...`. Comment lines
//     starting with `#` and the `empty=` sentinel are skipped.
//
// PURL canonical form per purl-spec/maven:
// `pkg:maven/<groupId>/<artifactId>@<version>`. Name on the
// PackageVersion stays as the conventional Maven coordinate
// `groupId:artifactId` so an IoC publisher who refers to a package
// by its build coordinate keys the same way.
//
// No Rust sidecar parity. Per ROADMAP, JVM ships single-engine until
// Wasm parity in v0.5.0+.
package maven

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

var ErrMalformedLockfile = errors.New("malformed lockfile")

// Format identifies which JVM lockfile dialect the caller is feeding
// the parser. Set by the CLI from filename
// (resolveEcosystem in cli/internal/commands/scan.go).
type Format string

const (
	FormatPom    Format = "pom"         // pom.xml
	FormatGradle Format = "gradle-lock" // gradle.lockfile
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
	case FormatPom:
		pkgs, err = parsePom(content)
		source = "maven-pom-v4"
	case FormatGradle:
		pkgs, err = parseGradleLock(content)
		source = "gradle-lockfile-v1"
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
		Ecosystem:    "maven",
		Packages:     pkgs,
		SourceFormat: source,
		SourceBytes:  int64(len(content)),
	}, nil
}

// pomDoc captures only the fields the matcher needs.
// `<dependencyManagement>` is intentionally NOT walked — those are
// version policies, not actual dependencies pulled into the build.
type pomDoc struct {
	XMLName    xml.Name `xml:"project"`
	Properties struct {
		Entries []pomProperty `xml:",any"`
	} `xml:"properties"`
	Dependencies struct {
		Dependency []pomDep `xml:"dependency"`
	} `xml:"dependencies"`
}

type pomProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type pomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
}

func parsePom(content []byte) ([]domain.PackageVersion, error) {
	var doc pomDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("%w: pom.xml decode: %v", ErrMalformedLockfile, err)
	}
	props := map[string]string{}
	for _, p := range doc.Properties.Entries {
		props[p.XMLName.Local] = strings.TrimSpace(p.Value)
	}

	pkgs := make([]domain.PackageVersion, 0, len(doc.Dependencies.Dependency))
	seen := map[string]bool{}
	for _, d := range doc.Dependencies.Dependency {
		group := strings.TrimSpace(d.GroupID)
		artifact := strings.TrimSpace(d.ArtifactID)
		version := substituteProperty(strings.TrimSpace(d.Version), props)
		if group == "" || artifact == "" || version == "" {
			continue
		}
		coord := group + ":" + artifact
		key := coord + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		var scope []string
		if d.Scope != "" {
			scope = []string{d.Scope}
		}
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "maven",
			Name:      coord,
			Version:   version,
			PURL:      mavenPURL(group, artifact, version),
			Scope:     scope,
		})
	}
	return pkgs, nil
}

// substituteProperty resolves a single ${name} placeholder against the
// pom's <properties> map. If the placeholder cannot be resolved we
// return the original string — including the unresolved `${…}` —
// rather than returning empty, so the resulting SBOM keeps a visible
// trace of the unresolved binding for the operator to follow.
func substituteProperty(version string, props map[string]string) string {
	if !strings.HasPrefix(version, "${") || !strings.HasSuffix(version, "}") {
		return version
	}
	key := strings.TrimSuffix(strings.TrimPrefix(version, "${"), "}")
	if v, ok := props[key]; ok && v != "" {
		return v
	}
	return version
}

// parseGradleLock walks `gradle.lockfile`. Each non-comment line is
// `groupId:artifactId:version=<config-list>`. Lines containing
// `empty=` are sentinels Gradle writes when no dependencies exist for
// a configuration; they carry no coordinate and are skipped.
func parseGradleLock(content []byte) ([]domain.PackageVersion, error) {
	var pkgs []domain.PackageVersion
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seen := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.LastIndex(line, "=")
		if eq < 0 {
			continue
		}
		coord := line[:eq]
		// Skip `empty=…` and any line whose left side carries no colon
		// (not a coordinate — Gradle writes a few internal config
		// lines we have no reason to surface).
		if !strings.Contains(coord, ":") {
			continue
		}
		parts := strings.SplitN(coord, ":", 3)
		if len(parts) != 3 {
			continue
		}
		group := strings.TrimSpace(parts[0])
		artifact := strings.TrimSpace(parts[1])
		version := strings.TrimSpace(parts[2])
		if group == "" || artifact == "" || version == "" {
			continue
		}
		name := group + ":" + artifact
		key := name + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		pkgs = append(pkgs, domain.PackageVersion{
			Ecosystem: "maven",
			Name:      name,
			Version:   version,
			PURL:      mavenPURL(group, artifact, version),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: gradle lockfile scan: %v", ErrMalformedLockfile, err)
	}
	return pkgs, nil
}

// mavenPURL builds the purl-spec form. Group + artifact stay
// unescaped since Maven coordinates use only ASCII identifier chars
// in practice; any pathological case (rare in published artifacts)
// would land on the JOIN side as the same literal anyway.
func mavenPURL(group, artifact, version string) string {
	return fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, version)
}
