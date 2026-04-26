package gomod_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/sbom/gomod"
)

func TestParser_HappyPath_TwoModules(t *testing.T) {
	gosum := []byte(
		"github.com/spf13/cobra v1.8.0 h1:hashA=\n" +
			"github.com/spf13/cobra v1.8.0/go.mod h1:hashAmod=\n" +
			"github.com/stretchr/testify v1.9.0 h1:hashB=\n" +
			"github.com/stretchr/testify v1.9.0/go.mod h1:hashBmod=\n",
	)
	gomodFile := []byte("module example.com/x\n\ngo 1.21\n")

	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, gomodFile)
	require.NoError(t, err)
	require.Equal(t, "gomod", parsed.Ecosystem)
	require.Equal(t, "go-sum-v1", parsed.SourceFormat)
	require.Equal(t, int64(len(gosum)), parsed.SourceBytes)
	require.Len(t, parsed.Packages, 2)

	require.Equal(t, "github.com/spf13/cobra", parsed.Packages[0].Name)
	require.Equal(t, "v1.8.0", parsed.Packages[0].Version)
	require.Equal(t, "h1:hashA=", parsed.Packages[0].Integrity)
	require.Equal(t, "pkg:golang/github.com/spf13/cobra@v1.8.0", parsed.Packages[0].PURL)
	require.Nil(t, parsed.Packages[0].Scope)
}

func TestParser_PseudoVersionPassthrough(t *testing.T) {
	gosum := []byte(
		"github.com/foo/bar v0.0.0-20240115123456-abcdef123456 h1:p=\n" +
			"github.com/foo/bar v0.0.0-20240115123456-abcdef123456/go.mod h1:pm=\n",
	)
	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, nil)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, "v0.0.0-20240115123456-abcdef123456", parsed.Packages[0].Version)
}

func TestParser_RemoteReplaceSubstitutesTarget(t *testing.T) {
	// In real Go projects the replacement appears in go.sum under the
	// new module path. Mirror that here.
	gosum := []byte(
		"github.com/new/lib v2.0.0 h1:nh=\n" +
			"github.com/new/lib v2.0.0/go.mod h1:nhm=\n",
	)
	gomodFile := []byte("module x\nreplace github.com/old/lib => github.com/new/lib v2.0.0\n")
	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, gomodFile)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, "github.com/new/lib", parsed.Packages[0].Name)
	require.Equal(t, "v2.0.0", parsed.Packages[0].Version)
}

func TestParser_LocalReplaceDropsSource(t *testing.T) {
	gosum := []byte(
		"github.com/old/lib v1.0.0 h1:o=\n" +
			"github.com/old/lib v1.0.0/go.mod h1:om=\n",
	)
	gomodFile := []byte("module x\nreplace github.com/old/lib => ./local-fork\n")
	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, gomodFile)
	require.NoError(t, err)
	require.Empty(t, parsed.Packages, "local replace must drop source entry")
}

func TestParser_ReplaceBlockForm(t *testing.T) {
	gosum := []byte(
		"github.com/new/a v1.0.0 h1:a=\n" +
			"github.com/new/a v1.0.0/go.mod h1:am=\n",
	)
	gomodFile := []byte("module x\n\nreplace (\n\tgithub.com/old/a => github.com/new/a v1.0.0\n\tgithub.com/old/b => ./fork\n)\n")
	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, gomodFile)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, "github.com/new/a", parsed.Packages[0].Name)
}

func TestParser_RejectsMalformedLine(t *testing.T) {
	gosum := []byte("github.com/foo/bar v1.0.0\n") // missing hash field
	_, err := gomod.NewParser().Parse(context.Background(), gosum, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, gomod.ErrMalformedLockfile))
}

func TestParser_RejectsNonH1Hash(t *testing.T) {
	gosum := []byte("github.com/foo/bar v1.0.0 sha256:abc=\n")
	_, err := gomod.NewParser().Parse(context.Background(), gosum, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, gomod.ErrMalformedLockfile))
}

func TestParser_EmptyGosumYieldsEmptyPackages(t *testing.T) {
	parsed, err := gomod.NewParser().Parse(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Empty(t, parsed.Packages)
	require.Equal(t, "gomod", parsed.Ecosystem)
}

func TestParser_DeterministicOrder(t *testing.T) {
	// Insert in non-alphabetical order; output must sort by (Name, Version).
	gosum := []byte(
		"github.com/zzz/last v1.0.0 h1:z=\n" +
			"github.com/zzz/last v1.0.0/go.mod h1:zm=\n" +
			"github.com/aaa/first v0.5.0 h1:a=\n" +
			"github.com/aaa/first v0.5.0/go.mod h1:am=\n" +
			"github.com/aaa/first v0.4.0 h1:a4=\n" +
			"github.com/aaa/first v0.4.0/go.mod h1:a4m=\n",
	)
	parsed, err := gomod.NewParser().Parse(context.Background(), gosum, nil)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 3)
	require.Equal(t, "github.com/aaa/first", parsed.Packages[0].Name)
	require.Equal(t, "v0.4.0", parsed.Packages[0].Version)
	require.Equal(t, "v0.5.0", parsed.Packages[1].Version)
	require.Equal(t, "github.com/zzz/last", parsed.Packages[2].Name)
}
