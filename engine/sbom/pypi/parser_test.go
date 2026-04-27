package pypi_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/sbom/pypi"
)

func TestParser_Requirements_HappyPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "pypi",
		"simple-requirements", "requirements.txt"))
	require.NoError(t, err)

	parsed, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatRequirements)
	require.NoError(t, err)
	require.Equal(t, "pypi", parsed.Ecosystem)
	require.Equal(t, "requirements-txt-v1", parsed.SourceFormat)
	require.Equal(t, int64(len(body)), parsed.SourceBytes)

	// 7 lines have a `==` pin: Django, djangorestframework, celery,
	// psycopg2-binary, gunicorn, requests, urllib3.
	// flask>=2.0.0 (range), git+, -e, --hash all skipped.
	require.Len(t, parsed.Packages, 7)

	// Names normalised to PEP-503 lowercase with single-dash collapse.
	names := make([]string, 0, len(parsed.Packages))
	for _, p := range parsed.Packages {
		names = append(names, p.Name)
		require.Equal(t, "pypi", p.Ecosystem)
	}
	require.ElementsMatch(t, []string{
		"celery", "django", "djangorestframework", "gunicorn",
		"psycopg2-binary", "requests", "urllib3",
	}, names)

	// Inline-comment + extras stripped, version retained.
	for _, p := range parsed.Packages {
		if p.Name == "celery" {
			require.Equal(t, "5.3.4", p.Version, "extras must not bleed into the version")
		}
		if p.Name == "urllib3" {
			require.Equal(t, "2.0.7", p.Version, "post-version environment markers must be stripped")
		}
	}
}

func TestParser_Requirements_NormalisesPEP503Name(t *testing.T) {
	body := []byte("Some_Funky.Package==1.0.0\n")
	parsed, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatRequirements)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
	require.Equal(t, "some-funky-package", parsed.Packages[0].Name,
		"PEP 503 collapses _, ., - and lowercases")
	require.Equal(t, "pkg:pypi/some-funky-package@1.0.0", parsed.Packages[0].PURL)
}

func TestParser_Requirements_DedupesIdenticalPin(t *testing.T) {
	body := []byte("requests==2.31.0\nrequests==2.31.0\n")
	parsed, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatRequirements)
	require.NoError(t, err)
	require.Len(t, parsed.Packages, 1)
}

func TestParser_PoetryLock_HappyPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "pypi",
		"poetry-lock", "poetry.lock"))
	require.NoError(t, err)

	parsed, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatPoetry)
	require.NoError(t, err)
	require.Equal(t, "pypi", parsed.Ecosystem)
	require.Equal(t, "poetry-lock-v2", parsed.SourceFormat)
	require.Len(t, parsed.Packages, 5,
		"certifi, charset-normalizer, idna, requests, urllib3")

	for _, p := range parsed.Packages {
		require.NotEmpty(t, p.Version)
		require.Equal(t, "pypi", p.Ecosystem)
	}
}

func TestParser_UVLock_HappyPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lockfiles", "pypi",
		"uv-lock", "uv.lock"))
	require.NoError(t, err)

	parsed, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatUV)
	require.NoError(t, err)
	require.Equal(t, "pypi", parsed.Ecosystem)
	require.Equal(t, "uv-lock-v1", parsed.SourceFormat)
	require.Len(t, parsed.Packages, 7,
		"anyio, fastapi, idna, pydantic, sniffio, starlette, uvicorn")
}

func TestParser_UnknownFormat_Error(t *testing.T) {
	_, err := pypi.NewParser().Parse(context.Background(), []byte("anything"), pypi.Format("nonsense"))
	require.Error(t, err)
	require.True(t, errors.Is(err, pypi.ErrMalformedLockfile))
}

func TestParser_MalformedTOML_Error(t *testing.T) {
	body := []byte("[[package\nname = \"x\"")
	_, err := pypi.NewParser().Parse(context.Background(), body, pypi.FormatPoetry)
	require.Error(t, err)
	require.True(t, errors.Is(err, pypi.ErrMalformedLockfile))
}
