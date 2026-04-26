package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// PackageVersion is a canonical ecosystem/name/version identifier for a dependency.
type PackageVersion struct {
	Ecosystem string
	Name      string
	Version   string
	PURL      string
	Scope     []string
	Integrity string
}

// CanonicalPURL returns a purl (package URL) for ecosystem/name/version.
// Scoped npm names have their leading '@' URL-encoded per the purl spec.
//
// Internal ecosystem names map to purl-spec types where they differ:
//   - "gomod" → "pkg:golang/..." (purl spec uses "golang", not "gomod")
//   - "cargo" → "pkg:cargo/..." (1:1)
//   - "npm"   → "pkg:npm/..."  (with @scope encoding)
func CanonicalPURL(ecosystem, name, version string) string {
	switch ecosystem {
	case "npm":
		return npmPURL(name, version)
	case "gomod":
		return fmt.Sprintf("pkg:golang/%s@%s", name, version)
	default:
		return fmt.Sprintf("pkg:%s/%s@%s", ecosystem, name, version)
	}
}

func npmPURL(name, version string) string {
	if strings.HasPrefix(name, "@") {
		rest := name[1:]
		if slash := strings.Index(rest, "/"); slash > 0 {
			ns := rest[:slash]
			pkg := rest[slash+1:]
			return fmt.Sprintf("pkg:npm/%s/%s@%s", url.QueryEscape("@"+ns), pkg, version)
		}
	}
	return fmt.Sprintf("pkg:npm/%s@%s", name, version)
}
