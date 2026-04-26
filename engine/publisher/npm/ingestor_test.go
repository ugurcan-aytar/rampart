package npm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
	"github.com/ugurcan-aytar/rampart/engine/publisher/npm"
)

const axiosResponse = `{
  "name": "axios",
  "dist-tags": { "latest": "1.11.0" },
  "maintainers": [
    { "email": "matt@example.com", "name": "mzabriskie" }
  ],
  "time": {
    "1.10.0": "2026-03-01T10:00:00.000Z",
    "1.11.0": "2026-03-31T12:00:00.000Z"
  },
  "versions": {
    "1.11.0": {
      "dist": {
        "signatures": [{ "keyid": "abc", "sig": "xyz" }]
      }
    }
  },
  "repository": "git+https://github.com/axios/axios.git"
}`

const tokenPublishedResponse = `{
  "name": "legacy-pkg",
  "dist-tags": { "latest": "1.0.0" },
  "maintainers": [{ "email": "x@example.com", "name": "x" }],
  "time": { "1.0.0": "2024-01-01T00:00:00.000Z" },
  "versions": { "1.0.0": { "dist": {} } },
  "repository": { "type": "git", "url": "https://github.com/x/y" }
}`

func TestIngest_HappyPath_OIDCSignals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/axios", r.URL.Path)
		require.Contains(t, r.Header.Get("Accept"), "application/")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(axiosResponse))
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL, Timeout: 2 * time.Second})

	snap, err := ing.Ingest(context.Background(), "npm:axios")
	require.NoError(t, err)
	require.Equal(t, "npm:axios", snap.PackageRef)
	require.Equal(t, "1.11.0", snap.LatestVersion)
	require.NotNil(t, snap.LatestVersionPublishedAt)
	require.Equal(t, "2026-03-31T12:00:00Z", snap.LatestVersionPublishedAt.Format(time.RFC3339))
	require.Equal(t, "oidc-trusted-publisher", snap.PublishMethod)
	require.Len(t, snap.Maintainers, 1)
	require.Equal(t, "matt@example.com", snap.Maintainers[0].Email)
	require.NotNil(t, snap.SourceRepoURL)
	require.Equal(t, "https://github.com/axios/axios.git", *snap.SourceRepoURL)
	require.NotEmpty(t, snap.RawData, "raw API body must be retained")
}

func TestIngest_TokenPublish_NoSignatures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tokenPublishedResponse))
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL})
	snap, err := ing.Ingest(context.Background(), "npm:legacy-pkg")
	require.NoError(t, err)
	require.Equal(t, "token", snap.PublishMethod, "no signatures → token publish")
	// Repository as object form.
	require.NotNil(t, snap.SourceRepoURL)
	require.Equal(t, "https://github.com/x/y", *snap.SourceRepoURL)
}

func TestIngest_ScopedPackage_KeepsSlashLiteral(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"@scope/pkg","dist-tags":{}}`))
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL})
	_, err := ing.Ingest(context.Background(), "npm:@scope/pkg")
	require.NoError(t, err)
	require.Equal(t, "/@scope/pkg", seen, "scoped name must travel as @scope/pkg, not %2F-encoded")
}

func TestIngest_NotFoundBubblesSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL})
	_, err := ing.Ingest(context.Background(), "npm:does-not-exist")
	require.Error(t, err)
	require.True(t, errors.Is(err, httpx.ErrNotFound))
}

func TestIngest_RateLimit429TriggersBackoff(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0") // back off immediately
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"x","dist-tags":{}}`))
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL})
	snap, err := ing.Ingest(context.Background(), "npm:x")
	require.NoError(t, err)
	require.Equal(t, int32(2), hits.Load(), "must retry after 429")
	require.Equal(t, "npm:x", snap.PackageRef)
}

func TestIngest_RejectsUnprefixedRef(t *testing.T) {
	ing := npm.New(npm.Config{RegistryBase: "http://unused"})
	_, err := ing.Ingest(context.Background(), "axios") // missing "npm:" prefix
	require.Error(t, err)
	require.Contains(t, err.Error(), "not npm:-prefixed")
}

func TestIngest_DecodeErrorOnNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	ing := npm.New(npm.Config{RegistryBase: srv.URL})
	_, err := ing.Ingest(context.Background(), "npm:x")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "decode response"))
}
