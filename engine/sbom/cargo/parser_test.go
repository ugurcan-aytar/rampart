package cargo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/sbom/cargo"
)

func TestParser_HappyPath_RegistrySources(t *testing.T) {
	content := []byte(`
version = 4

[[package]]
name = "serde"
version = "1.0.215"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "abc123"

[[package]]
name = "thiserror"
version = "2.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "def456"
`)
	parsed, err := cargo.NewParser().Parse(context.Background(), content)
	require.NoError(t, err)
	require.Equal(t, "cargo", parsed.Ecosystem)
	require.Equal(t, "cargo-lock-v3", parsed.SourceFormat)
	require.Equal(t, int64(len(content)), parsed.SourceBytes)
	require.Len(t, parsed.Packages, 2)

	require.Equal(t, "serde", parsed.Packages[0].Name)
	require.Equal(t, "1.0.215", parsed.Packages[0].Version)
	require.Equal(t, "abc123", parsed.Packages[0].Integrity)
	require.Equal(t, "pkg:cargo/serde@1.0.215", parsed.Packages[0].PURL)
	require.Nil(t, parsed.Packages[0].Scope)
}

func TestParser_SkipsWorkspaceMember(t *testing.T) {
	content := []byte(`
version = 4

[[package]]
name = "rampart-native"
version = "0.1.0"

[[package]]
name = "serde"
version = "1.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
checksum = "x"
`)
	parsed, err := cargo.NewParser().Parse(context.Background(), content)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1, "workspace member must be skipped")
	require.Equal(t, "serde", parsed.Packages[0].Name)
}

func TestParser_GitSourceTagged(t *testing.T) {
	content := []byte(`
[[package]]
name = "exotic"
version = "0.1.0"
source = "git+https://github.com/rust-lang/exotic?branch=main#abcdef0123"
`)
	parsed, err := cargo.NewParser().Parse(context.Background(), content)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, []string{"git"}, parsed.Packages[0].Scope)
	require.Empty(t, parsed.Packages[0].Integrity)
}

func TestParser_RejectsMalformedTOML(t *testing.T) {
	_, err := cargo.NewParser().Parse(context.Background(), []byte("[[package]\nname = "))
	require.Error(t, err)
	require.True(t, errors.Is(err, cargo.ErrMalformedLockfile))
}

func TestParser_RejectsEmptyPackages(t *testing.T) {
	_, err := cargo.NewParser().Parse(context.Background(), []byte("version = 4\n"))
	require.Error(t, err)
	require.True(t, errors.Is(err, cargo.ErrEmptyLockfile))
}

func TestParser_DeterministicOrder(t *testing.T) {
	content := []byte(`
[[package]]
name = "zzz"
version = "1.0.0"
source = "registry+x"

[[package]]
name = "aaa"
version = "0.5.0"
source = "registry+x"

[[package]]
name = "aaa"
version = "0.4.0"
source = "registry+x"
`)
	parsed, err := cargo.NewParser().Parse(context.Background(), content)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 3)
	require.Equal(t, "aaa", parsed.Packages[0].Name)
	require.Equal(t, "0.4.0", parsed.Packages[0].Version)
	require.Equal(t, "0.5.0", parsed.Packages[1].Version)
	require.Equal(t, "zzz", parsed.Packages[2].Name)
}
