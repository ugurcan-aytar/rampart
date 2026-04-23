package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
)

const testSecret = "test-secret-do-not-use-in-prod"

func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := middleware.PrincipalFromContext(r.Context()); ok {
			w.Header().Set("X-Subject", p.Subject)
			w.Header().Set("X-Scope", string(p.Scope))
		}
		w.WriteHeader(http.StatusOK)
	})
}

func enabledOptions() middleware.AuthOptions {
	return middleware.AuthOptions{
		Enabled:     true,
		SigningKey:  testSecret,
		Algorithm:   "HS256",
		Audience:    "rampart-test",
		ExemptPaths: middleware.DefaultExemptPaths,
	}
}

func TestAuthDisabledPassesThrough(t *testing.T) {
	opts := middleware.AuthOptions{Enabled: false}
	h := middleware.Auth(opts)(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthExemptPathsSkipValidation(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	for _, path := range []string{"/healthz", "/readyz", "/v1/stream", "/v1/auth/token"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equalf(t, http.StatusOK, rec.Code, "path %s should be exempt", path)
	}
}

func TestAuthMissingTokenRejected(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), `Bearer`)
}

func TestAuthValidTokenAttachesPrincipal(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	tok, _, err := middleware.IssueHS256(testSecret, "alice", "rampart-test", middleware.ScopeWrite, time.Minute, time.Now())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "alice", rec.Header().Get("X-Subject"))
	assert.Equal(t, "write", rec.Header().Get("X-Scope"))
}

func TestAuthExpiredTokenRejected(t *testing.T) {
	opts := enabledOptions()
	h := middleware.Auth(opts)(echoHandler())

	past := time.Now().Add(-2 * time.Hour)
	tok, _, err := middleware.IssueHS256(testSecret, "alice", "rampart-test", middleware.ScopeWrite, time.Minute, past)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthWrongAudienceRejected(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	tok, _, err := middleware.IssueHS256(testSecret, "alice", "some-other-service", middleware.ScopeWrite, time.Minute, time.Now())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthWrongSignatureRejected(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	tok, _, err := middleware.IssueHS256("a-different-secret", "alice", "rampart-test", middleware.ScopeWrite, time.Minute, time.Now())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMalformedBearerRejected(t *testing.T) {
	h := middleware.Auth(enabledOptions())(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthRS256Roundtrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   "svc:catalog-sync",
		Audience:  jwt.ClaimStrings{"rampart-rs"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(priv)
	require.NoError(t, err)

	opts := middleware.AuthOptions{
		Enabled:     true,
		SigningKey:  string(pubPEM),
		Algorithm:   "RS256",
		Audience:    "rampart-rs",
		ExemptPaths: middleware.DefaultExemptPaths,
	}
	h := middleware.Auth(opts)(echoHandler())
	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "svc:catalog-sync", rec.Header().Get("X-Subject"))
}

func TestAuthAlgorithmMismatchRejected(t *testing.T) {
	// Token is HS256-signed; middleware expects RS256 → parser rejects before keyFunc runs.
	opts := enabledOptions()
	opts.Algorithm = "RS256"
	opts.SigningKey = "-----BEGIN PUBLIC KEY-----\nMIIBIjAN\n-----END PUBLIC KEY-----\n"
	h := middleware.Auth(opts)(echoHandler())

	tok, _, err := middleware.IssueHS256(testSecret, "alice", "rampart-test", middleware.ScopeWrite, time.Minute, time.Now())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
