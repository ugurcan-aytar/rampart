package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORS_AllowAllEchoesOrigin(t *testing.T) {
	h := middleware.CORS(middleware.DefaultCORSOptions())(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://app.example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORS_AllowAllWithoutOriginUsesWildcard(t *testing.T) {
	h := middleware.CORS(middleware.DefaultCORSOptions())(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowListAcceptsOrigin(t *testing.T) {
	opts := middleware.CORSOptions{
		AllowAll: false,
		Origins:  []string{"https://app.example.com", "https://backstage.example.com"},
	}
	h := middleware.CORS(opts)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Origin", "https://backstage.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://backstage.example.com", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowListRejectsForeignOrigin(t *testing.T) {
	opts := middleware.CORSOptions{
		AllowAll: false,
		Origins:  []string{"https://app.example.com"},
	}
	h := middleware.CORS(opts)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "request still hits handler; the browser enforces the block")
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"),
		"foreign origin must not get an allow-origin header back")
}

func TestCORS_PreflightReturns204(t *testing.T) {
	h := middleware.CORS(middleware.DefaultCORSOptions())(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/v1/components", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "https://app.example.com", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ServerToServerRequestUnaffected(t *testing.T) {
	// CLI / CI / backend-proxy traffic ships no Origin header. The
	// middleware should just pass through — no allow-origin header
	// required, no 204 returned.
	opts := middleware.CORSOptions{AllowAll: false, Origins: []string{"https://app.example.com"}}
	h := middleware.CORS(opts)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_EmptyOriginsWithAllowAllFalseBlocksAll(t *testing.T) {
	opts := middleware.CORSOptions{AllowAll: false}
	h := middleware.CORS(opts)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/components", nil)
	req.Header.Set("Origin", "https://any.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}
