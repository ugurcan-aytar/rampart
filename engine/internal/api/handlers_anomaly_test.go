package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

func newAnomalyTestHandler(t *testing.T) (http.Handler, *memory.Store) {
	t.Helper()
	store := memory.New()
	t.Cleanup(func() { _ = store.Close() })
	bus := events.NewBus(64)
	srv := api.NewServer(store, bus, 0)
	return srv.Handler(), store
}

func seedAnomalies(t *testing.T, store *memory.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	for _, a := range []domain.Anomaly{
		{
			Kind:        domain.AnomalyKindMaintainerEmailDrift,
			PackageRef:  "npm:axios",
			DetectedAt:  now,
			Confidence:  domain.ConfidenceHigh,
			Explanation: "evil@bad.io appeared",
			Evidence:    map[string]any{"new_emails": []any{"evil@bad.io"}},
		},
		{
			Kind:        domain.AnomalyKindOIDCPublishingRegression,
			PackageRef:  "npm:axios",
			DetectedAt:  now.Add(-2 * time.Hour),
			Confidence:  domain.ConfidenceHigh,
			Explanation: "OIDC → token",
			Evidence:    map[string]any{},
		},
		{
			Kind:        domain.AnomalyKindVersionJump,
			PackageRef:  "gomod:github.com/spf13/cobra",
			DetectedAt:  now.Add(-time.Hour),
			Confidence:  domain.ConfidenceHigh,
			Explanation: "v1.0.6 → v48.0.0",
			Evidence:    map[string]any{},
		},
	} {
		_, err := store.SaveAnomaly(ctx, a)
		require.NoError(t, err)
	}
}

func TestListAnomalies_NewestFirst(t *testing.T) {
	h, store := newAnomalyTestHandler(t)
	seedAnomalies(t, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/anomalies", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp gen.AnomalyPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 3)
	for i := 0; i+1 < len(resp.Items); i++ {
		require.False(t, resp.Items[i].DetectedAt.Before(resp.Items[i+1].DetectedAt),
			"newest-first ordering")
	}
}

func TestListAnomalies_FilterByPackageRef(t *testing.T) {
	h, store := newAnomalyTestHandler(t)
	seedAnomalies(t, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/anomalies?package_ref=npm:axios", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp gen.AnomalyPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	for _, a := range resp.Items {
		require.Equal(t, "npm:axios", a.PackageRef)
	}
}

func TestListAnomalies_FilterByKind(t *testing.T) {
	h, store := newAnomalyTestHandler(t)
	seedAnomalies(t, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/anomalies?kind=version_jump", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp gen.AnomalyPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	require.Equal(t, gen.AnomalyKindVersionJump, resp.Items[0].Kind)
}

func TestListAnomalies_LimitClamps(t *testing.T) {
	h, store := newAnomalyTestHandler(t)
	seedAnomalies(t, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/anomalies?limit=2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp gen.AnomalyPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
}

func TestGetAnomaly_HappyPath(t *testing.T) {
	h, store := newAnomalyTestHandler(t)
	id, err := store.SaveAnomaly(context.Background(), domain.Anomaly{
		Kind:        domain.AnomalyKindMaintainerEmailDrift,
		PackageRef:  "npm:axios",
		DetectedAt:  time.Now().UTC(),
		Confidence:  domain.ConfidenceHigh,
		Explanation: "evil@bad.io appeared",
		Evidence:    map[string]any{"foo": "bar"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/anomalies/%d", id), nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var got gen.Anomaly
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, id, got.Id)
	require.Equal(t, gen.AnomalyKindNewMaintainerEmail, got.Kind)
	require.NotNil(t, got.Evidence)
	require.Equal(t, "bar", (*got.Evidence)["foo"])
}

func TestGetAnomaly_NotFound(t *testing.T) {
	h, _ := newAnomalyTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/anomalies/999999", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
}
