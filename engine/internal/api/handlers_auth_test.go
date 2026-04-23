package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

const authTestSecret = "server-test-secret"

func authEnabledServer(t *testing.T) *api.Server {
	t.Helper()
	s := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	s.SetAuth(middleware.AuthOptions{
		Enabled:     true,
		SigningKey:  authTestSecret,
		Algorithm:   "HS256",
		Audience:    "rampart-test",
		ExemptPaths: middleware.DefaultExemptPaths,
	})
	return s
}

func TestIssueAuthToken_Success(t *testing.T) {
	srv := authEnabledServer(t)
	body, _ := json.Marshal(gen.AuthTokenRequest{Subject: "svc:alice"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp gen.AuthTokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.AccessToken)
	require.True(t, resp.ExpiresAt.After(time.Now()))
}

func TestIssueAuthToken_DisabledReturns503(t *testing.T) {
	srv := api.NewServer(memory.New(), events.NewBus(16), time.Minute)
	body, _ := json.Marshal(gen.AuthTokenRequest{Subject: "svc:alice"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	require.Contains(t, rr.Body.String(), "AUTH_DISABLED")
}

func TestAuthEnabled_UnauthenticatedCallRejected(t *testing.T) {
	srv := authEnabledServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthEnabled_IssuedTokenIsAccepted(t *testing.T) {
	srv := authEnabledServer(t)

	issueBody, _ := json.Marshal(gen.AuthTokenRequest{Subject: "svc:alice"})
	issueReq := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bytes.NewReader(issueBody))
	issueReq.Header.Set("Content-Type", "application/json")
	issueRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(issueRR, issueReq)
	require.Equal(t, http.StatusOK, issueRR.Code)

	var tok gen.AuthTokenResponse
	require.NoError(t, json.Unmarshal(issueRR.Body.Bytes(), &tok))

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestAuthEnabled_HealthAndStreamRemainOpen(t *testing.T) {
	srv := authEnabledServer(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		require.Equalf(t, http.StatusOK, rr.Code, "path %s should pass without auth", path)
	}
}
