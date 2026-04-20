package npm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

const fixturesDir = "../../testdata/lockfiles"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixturesDir, name))
	require.NoError(t, err)
	return b
}

func TestParse_AxiosCompromise(t *testing.T) {
	p := npm.NewParser()
	s, err := p.Parse(context.Background(), readFixture(t, "axios-compromise.json"), npm.LockfileMeta{
		ComponentRef: "kind:Component/default/vulnerable-app",
		CommitSHA:    "abc123",
	})
	require.NoError(t, err)
	require.Equal(t, "npm", s.Ecosystem)
	require.Equal(t, "npm-package-lock-v3", s.SourceFormat)
	require.Equal(t, "kind:Component/default/vulnerable-app", s.ComponentRef)
	require.Equal(t, "abc123", s.CommitSHA)

	byName := map[string]domain.PackageVersion{}
	for _, pkg := range s.Packages {
		byName[pkg.Name] = pkg
	}
	axios, ok := byName["axios"]
	require.True(t, ok, "axios must be present")
	require.Equal(t, "1.11.0", axios.Version)
	require.Equal(t, "pkg:npm/axios@1.11.0", axios.PURL)

	crypto, ok := byName["plain-crypto-js"]
	require.True(t, ok, "plain-crypto-js (typosquat) must be present")
	require.Equal(t, "4.2.1", crypto.Version)
	require.Equal(t, "pkg:npm/plain-crypto-js@4.2.1", crypto.PURL)
}

func TestParse_SimpleWebapp(t *testing.T) {
	p := npm.NewParser()
	s, err := p.Parse(context.Background(), readFixture(t, "simple-webapp.json"), npm.LockfileMeta{})
	require.NoError(t, err)
	require.NotEmpty(t, s.Packages)

	byName := map[string]domain.PackageVersion{}
	for _, pkg := range s.Packages {
		byName[pkg.Name] = pkg
	}

	// Prod deps — no scope markers.
	require.Contains(t, byName, "react")
	require.Nil(t, byName["react"].Scope, "prod deps have no scope markers")

	// Dev dep.
	require.Contains(t, byName, "@types/react")
	require.Equal(t, []string{"dev"}, byName["@types/react"].Scope)
	require.Equal(t, "pkg:npm/%40types/react@18.2.0", byName["@types/react"].PURL,
		"scoped package must URL-encode the leading @")

	// Multi-scope (dev + peer).
	require.Contains(t, byName, "@types/react-dom")
	require.Equal(t, []string{"dev", "peer"}, byName["@types/react-dom"].Scope)

	// Optional dep.
	require.Contains(t, byName, "fsevents")
	require.Equal(t, []string{"optional"}, byName["fsevents"].Scope)

	// Deterministic ordering (parity with the Rust parser in Adım 6).
	for i := 1; i < len(s.Packages); i++ {
		prev := s.Packages[i-1]
		cur := s.Packages[i]
		if prev.Name == cur.Name {
			require.LessOrEqual(t, prev.Version, cur.Version)
		} else {
			require.Less(t, prev.Name, cur.Name)
		}
	}
}

func TestParse_Workspaces(t *testing.T) {
	p := npm.NewParser()
	s, err := p.Parse(context.Background(), readFixture(t, "with-workspaces.json"), npm.LockfileMeta{})
	require.NoError(t, err)

	// Workspace source paths ("packages/app", "packages/lib") and their
	// node_modules symlinks (link: true) must be filtered — we don't want
	// them inflating the SBOM.
	for _, pkg := range s.Packages {
		require.NotEqual(t, "@scope/app", pkg.Name, "workspace link must not appear")
		require.NotEqual(t, "@scope/lib", pkg.Name)
	}

	// The real third-party dep survives.
	byName := map[string]domain.PackageVersion{}
	for _, pkg := range s.Packages {
		byName[pkg.Name] = pkg
	}
	dep, ok := byName["lodash-es"]
	require.True(t, ok, "third-party dep lodash-es must survive workspace filtering")
	require.Equal(t, "4.17.21", dep.Version)
	require.Equal(t, 1, len(s.Packages), "workspace fixture should yield exactly 1 third-party dep")
}

func TestParse_Scoped(t *testing.T) {
	p := npm.NewParser()
	s, err := p.Parse(context.Background(), readFixture(t, "with-scoped.json"), npm.LockfileMeta{})
	require.NoError(t, err)

	byPURL := map[string]bool{}
	for _, pkg := range s.Packages {
		byPURL[pkg.PURL] = true
	}
	require.True(t, byPURL["pkg:npm/%40types/node@22.0.0"], "scoped @types/node must be URL-encoded")
	require.True(t, byPURL["pkg:npm/%40backstage/core-components@0.15.0"], "scoped @backstage/... must be URL-encoded")
	require.True(t, byPURL["pkg:npm/plain-package@1.0.0"], "non-scoped must remain unencoded")
}

func TestParse_Empty(t *testing.T) {
	p := npm.NewParser()
	s, err := p.Parse(context.Background(), readFixture(t, "minimal.json"), npm.LockfileMeta{})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.Packages), "only-root lockfile yields zero installed packages")
}

func TestParse_MalformedJSON(t *testing.T) {
	p := npm.NewParser()
	_, err := p.Parse(context.Background(), readFixture(t, "malformed.json"), npm.LockfileMeta{})
	require.Error(t, err)
	require.True(t, errors.Is(err, npm.ErrMalformedLockfile))
}

func TestParse_UnsupportedVersion(t *testing.T) {
	p := npm.NewParser()
	_, err := p.Parse(context.Background(), readFixture(t, "wrong-version.json"), npm.LockfileMeta{})
	require.Error(t, err)
	require.True(t, errors.Is(err, npm.ErrUnsupportedLockfileVersion))
}

func TestParse_PackagesAbsent(t *testing.T) {
	p := npm.NewParser()
	body := []byte(`{"name":"x","version":"1.0.0","lockfileVersion":3}`)
	_, err := p.Parse(context.Background(), body, npm.LockfileMeta{})
	require.Error(t, err)
	require.True(t, errors.Is(err, npm.ErrEmptyLockfile))
}

func TestParse_EmptyPackagesMap(t *testing.T) {
	// Differs from PackagesAbsent: the key is present but the map is empty.
	// Treating this as an error is unhelpful; return a 0-package SBOM.
	p := npm.NewParser()
	body := []byte(`{"name":"x","version":"1.0.0","lockfileVersion":3,"packages":{}}`)
	s, err := p.Parse(context.Background(), body, npm.LockfileMeta{})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.Packages))
}

func TestParse_ContextCancelled(t *testing.T) {
	p := npm.NewParser()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Parse(ctx, []byte(`{}`), npm.LockfileMeta{})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPURLCanonicalization(t *testing.T) {
	tests := []struct {
		name, version, want string
	}{
		{"axios", "1.11.0", "pkg:npm/axios@1.11.0"},
		{"@types/node", "22.0.0", "pkg:npm/%40types/node@22.0.0"},
		{"@backstage/core-components", "0.15.0", "pkg:npm/%40backstage/core-components@0.15.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, domain.CanonicalPURL("npm", tc.name, tc.version))
		})
	}
}
