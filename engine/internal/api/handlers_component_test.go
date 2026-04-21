package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

func newTestServer(t *testing.T) (http.Handler, *memory.Store) {
	t.Helper()
	store := memory.New()
	srv := api.NewServer(store, events.NewBus(16), time.Minute)
	return srv.Handler(), store
}

func TestUpsertComponent_NewReturns201(t *testing.T) {
	h, store := newTestServer(t)

	body := `{"ref":"kind:Component/default/web","kind":"Component","namespace":"default","name":"web","owner":"team-platform"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/components",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, "body=%s", rr.Body.String())

	var got gen.Component
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "kind:Component/default/web", got.Ref)
	require.Equal(t, "Component", got.Kind)
	require.Equal(t, "default", got.Namespace)
	require.Equal(t, "web", got.Name)
	require.NotNil(t, got.Owner)
	require.Equal(t, "team-platform", *got.Owner)

	stored, err := store.GetComponent(context.Background(), "kind:Component/default/web")
	require.NoError(t, err)
	require.Equal(t, "team-platform", stored.Owner)
}

func TestUpsertComponent_ExistingReturns200(t *testing.T) {
	h, _ := newTestServer(t)

	first := `{"ref":"kind:Component/default/web","kind":"Component","namespace":"default","name":"web","owner":"team-platform"}`
	req1 := httptest.NewRequest(http.MethodPost, "/v1/components", bytes.NewReader([]byte(first)))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	require.Equal(t, http.StatusCreated, rr1.Code)

	// Second POST with the same ref, different owner — expect 200 Update.
	second := `{"ref":"kind:Component/default/web","kind":"Component","namespace":"default","name":"web","owner":"team-security"}`
	req2 := httptest.NewRequest(http.MethodPost, "/v1/components", bytes.NewReader([]byte(second)))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)

	var got gen.Component
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &got))
	require.NotNil(t, got.Owner)
	require.Equal(t, "team-security", *got.Owner, "upsert must overwrite owner")
}

func TestUpsertComponent_MissingRef_400(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"kind":"Component","namespace":"default","name":"web"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/components", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "INVALID_PAYLOAD", e.Code)
}

func TestUpsertComponent_BadRef_400(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"ref":"not-a-valid-ref","kind":"Component","namespace":"x","name":"y"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/components", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	var e gen.Error
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &e))
	require.Equal(t, "INVALID_REF", e.Code)
}
