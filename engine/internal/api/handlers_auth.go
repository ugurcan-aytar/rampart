package api

import (
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
)

// IssueAuthToken mints a short-lived HS256 JWT for internal / test
// callers. Returns 503 when the engine has no signing key configured —
// production deployments typically front the engine with an external
// IdP and leave this endpoint disabled.
func (s *Server) IssueAuthToken(w http.ResponseWriter, r *http.Request) {
	if s.auth.SigningKey == "" {
		writeError(w, http.StatusServiceUnavailable, "AUTH_DISABLED",
			"token issuance disabled — RAMPART_AUTH_SIGNING_KEY not set")
		return
	}
	var req gen.AuthTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "subject is required")
		return
	}
	scope := middleware.ScopeWrite
	if req.Scope != nil {
		scope = middleware.Scope(*req.Scope)
	}
	ttl := 3600 * time.Second
	if req.TtlSeconds != nil {
		ttl = clampTTL(time.Duration(*req.TtlSeconds) * time.Second)
	}
	now := time.Now()
	signed, exp, err := middleware.IssueHS256(
		s.auth.SigningKey, req.Subject, s.auth.Audience, scope, ttl, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AUTH_SIGN_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.AuthTokenResponse{
		AccessToken: signed,
		ExpiresAt:   exp,
	})
}

func clampTTL(d time.Duration) time.Duration {
	const (
		minTTL = 60 * time.Second
		maxTTL = 24 * time.Hour
	)
	switch {
	case d < minTTL:
		return minTTL
	case d > maxTTL:
		return maxTTL
	default:
		return d
	}
}
