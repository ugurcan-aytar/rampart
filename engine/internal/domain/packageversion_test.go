package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestCanonicalPURL(t *testing.T) {
	tests := []struct {
		name      string
		ecosystem string
		pkg       string
		ver       string
		want      string
	}{
		{"plain npm", "npm", "axios", "1.11.0", "pkg:npm/axios@1.11.0"},
		{"scoped npm", "npm", "@types/node", "22.0.0", "pkg:npm/%40types/node@22.0.0"},
		{"scoped backstage", "npm", "@backstage/core", "0.15.0", "pkg:npm/%40backstage/core@0.15.0"},
		{"pypi plain", "pypi", "requests", "2.32.0", "pkg:pypi/requests@2.32.0"},
		{"golang module", "golang", "github.com/google/uuid", "1.6.0", "pkg:golang/github.com/google/uuid@1.6.0"},
		{"gomod maps to golang purl", "gomod", "github.com/spf13/cobra", "v1.8.0", "pkg:golang/github.com/spf13/cobra@v1.8.0"},
		{"cargo plain", "cargo", "serde", "1.0.215", "pkg:cargo/serde@1.0.215"},
		{"degenerate scope without slash", "npm", "@orphan", "1.0.0", "pkg:npm/@orphan@1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, domain.CanonicalPURL(tc.ecosystem, tc.pkg, tc.ver))
		})
	}
}

func TestPackageVersion_ZeroValue(t *testing.T) {
	// A zero PackageVersion should be safe to construct — fields populated by parsers.
	var pv domain.PackageVersion
	require.Empty(t, pv.Name)
	require.Nil(t, pv.Scope)
}
