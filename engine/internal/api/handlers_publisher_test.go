package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

func newPublisherTestHandler(t *testing.T) (http.Handler, *memory.Store) {
	t.Helper()
	store := memory.New()
	t.Cleanup(func() { _ = store.Close() })
	bus := events.NewBus(64)
	srv := api.NewServer(store, bus, 0)
	return srv.Handler(), store
}

func TestGetPublisherHistory_EmptyForUnknownRef(t *testing.T) {
	h, _ := newPublisherTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/publisher/"+url.PathEscape("npm:never-seen")+"/history", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp gen.PublisherHistory
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Empty(t, resp.Items)
}

func TestGetPublisherHistory_NewestFirst(t *testing.T) {
	h, store := newPublisherTestHandler(t)
	ctx := context.Background()

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-30 * time.Minute)
	pubAt := time.Now().Add(-90 * 24 * time.Hour)
	repoURL := "https://github.com/axios/axios"

	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef:    "npm:axios",
		SnapshotAt:    older,
		LatestVersion: "1.10.0",
		PublishMethod: "token",
	}))
	require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
		PackageRef:               "npm:axios",
		SnapshotAt:               newer,
		LatestVersion:            "1.11.0",
		LatestVersionPublishedAt: &pubAt,
		PublishMethod:            "oidc-trusted-publisher",
		Maintainers: []domain.Maintainer{
			{Email: "m@example.com", Name: "M", Username: "mtnance"},
		},
		SourceRepoURL: &repoURL,
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/publisher/"+url.PathEscape("npm:axios")+"/history", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp gen.PublisherHistory
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	require.Equal(t, "1.11.0", *resp.Items[0].LatestVersion, "newest-first ordering")
	require.NotNil(t, resp.Items[0].PublishMethod)
	require.Equal(t, gen.OidcTrustedPublisher, *resp.Items[0].PublishMethod)
	require.NotNil(t, resp.Items[0].SourceRepoURL)
	require.Equal(t, repoURL, *resp.Items[0].SourceRepoURL)
	require.NotNil(t, resp.Items[0].Maintainers)
	require.Len(t, *resp.Items[0].Maintainers, 1)
	require.Equal(t, "m@example.com", *(*resp.Items[0].Maintainers)[0].Email)
}

func TestGetPublisherHistory_LimitClampsResultCount(t *testing.T) {
	h, store := newPublisherTestHandler(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, store.SavePublisherSnapshot(ctx, domain.PublisherSnapshot{
			PackageRef: "npm:axios",
			SnapshotAt: time.Now().Add(-time.Duration(i+1) * time.Minute),
		}))
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/publisher/"+url.PathEscape("npm:axios")+"/history?limit=2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp gen.PublisherHistory
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
}
