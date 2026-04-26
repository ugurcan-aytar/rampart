package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/publisher/github"
	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
)

const cobraReleaseResponse = `{
  "tag_name": "v1.8.0",
  "name": "v1.8.0",
  "html_url": "https://github.com/spf13/cobra/releases/tag/v1.8.0",
  "published_at": "2026-02-15T11:30:00Z",
  "author": { "login": "spf13" }
}`

func TestParseGithubRef_Cases(t *testing.T) {
	cases := []struct {
		in            string
		owner         string
		repo          string
		expectFailure bool
	}{
		{"gomod:github.com/spf13/cobra", "spf13", "cobra", false},
		{"gomod:github.com/spf13/cobra/sub-pkg", "spf13", "cobra", false},
		{"github:foo/bar", "foo", "bar", false},
		{"gomod:gitlab.com/x/y", "", "", true},
		{"npm:axios", "", "", true},
		{"gomod:github.com/onlyowner", "", "", true},
		{"github:bare", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			owner, repo, err := github.ParseGithubRef(tc.in)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.owner, owner)
			require.Equal(t, tc.repo, repo)
		})
	}
}

func TestIngest_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/spf13/cobra/releases/latest", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cobraReleaseResponse))
	}))
	defer srv.Close()

	ing := github.New(github.Config{APIBase: srv.URL, Timeout: 2 * time.Second})

	snap, err := ing.Ingest(context.Background(), "gomod:github.com/spf13/cobra")
	require.NoError(t, err)
	require.Equal(t, "gomod:github.com/spf13/cobra", snap.PackageRef)
	require.Equal(t, "v1.8.0", snap.LatestVersion)
	require.NotNil(t, snap.LatestVersionPublishedAt)
	require.Equal(t, "2026-02-15T11:30:00Z", snap.LatestVersionPublishedAt.Format(time.RFC3339))
	require.Equal(t, "unknown", snap.PublishMethod)
	require.NotNil(t, snap.SourceRepoURL)
	require.Equal(t, "https://github.com/spf13/cobra", *snap.SourceRepoURL)
	require.Len(t, snap.Maintainers, 1)
	require.Equal(t, "spf13", snap.Maintainers[0].Username)
	require.NotEmpty(t, snap.RawData)
}

func TestIngest_TokenSetAsBearer(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cobraReleaseResponse))
	}))
	defer srv.Close()

	ing := github.New(github.Config{APIBase: srv.URL, Token: "ghp_xxx"})
	_, err := ing.Ingest(context.Background(), "github:spf13/cobra")
	require.NoError(t, err)
	require.Equal(t, "Bearer ghp_xxx", auth)
}

func TestIngest_NoTokenSendsNoAuth(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cobraReleaseResponse))
	}))
	defer srv.Close()

	ing := github.New(github.Config{APIBase: srv.URL})
	_, err := ing.Ingest(context.Background(), "github:spf13/cobra")
	require.NoError(t, err)
	require.Empty(t, auth)
}

func TestIngest_NotFoundReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ing := github.New(github.Config{APIBase: srv.URL, Token: "x"})
	_, err := ing.Ingest(context.Background(), "github:no/repo")
	require.Error(t, err)
	require.True(t, errors.Is(err, httpx.ErrNotFound))
}

func TestIngest_RateLimit429TriggersBackoff(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cobraReleaseResponse))
	}))
	defer srv.Close()

	ing := github.New(github.Config{APIBase: srv.URL, Token: "x"})
	snap, err := ing.Ingest(context.Background(), "github:spf13/cobra")
	require.NoError(t, err)
	require.Equal(t, int32(2), hits.Load())
	require.Equal(t, "v1.8.0", snap.LatestVersion)
}

func TestIngest_RejectsNonGithubRef(t *testing.T) {
	ing := github.New(github.Config{APIBase: "http://unused"})
	_, err := ing.Ingest(context.Background(), "npm:axios")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not point at github.com")
}
