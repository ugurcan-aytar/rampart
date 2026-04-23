// Package middleware is the HTTP middleware bag for the engine: JWT
// authentication and CORS live here. The auth middleware validates a
// JWT on every mutation route under `/v1/*` and attaches the resolved
// subject + scope to the request context. Endpoints that must remain
// unauthenticated (health probes, SSE — which relies on the browser
// EventSource API that cannot attach an Authorization header, and
// `/v1/auth/token` itself) are exempted by path prefix.
package middleware

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthOptions is the static configuration the auth middleware needs.
// SigningKey is either an HS256 shared secret (as []byte) or an RS256
// PEM-encoded public key — Algorithm decides which.
type AuthOptions struct {
	Enabled    bool
	SigningKey string
	Algorithm  string
	Audience   string
	// ExemptPaths is the ordered list of path prefixes that bypass the
	// middleware. Order-insensitive semantically; duplicates are fine.
	ExemptPaths []string
	// Now is injected for tests; defaults to time.Now.
	Now func() time.Time
}

// DefaultExemptPaths is the baseline set of routes that must remain
// open regardless of auth config. Callers typically pass this list
// through unmodified.
var DefaultExemptPaths = []string{
	"/healthz",
	"/readyz",
	"/v1/auth/token",
	"/v1/stream",
}

// Scope is the coarse-grained capability tier extracted from the
// `scope` claim. Unknown values are treated as ScopeRead.
type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
	ScopeAdmin Scope = "admin"
)

// Principal is what the middleware puts on the request context after a
// successful validation. Handlers retrieve it with PrincipalFromContext.
type Principal struct {
	Subject string
	Scope   Scope
}

type contextKey struct{}

// PrincipalFromContext returns the principal the middleware attached,
// or the zero value + false if auth was disabled / path was exempt.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

// Auth returns the JWT validation middleware. When opts.Enabled is
// false the middleware is a no-op passthrough — this is the v0.1.x
// backward-compat path so existing `make demo-axios` flows keep
// working without a signing key configured.
func Auth(opts AuthOptions) func(http.Handler) http.Handler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !opts.Enabled || isExempt(r.URL.Path, opts.ExemptPaths) {
				next.ServeHTTP(w, r)
				return
			}
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "AUTH_MISSING", "Authorization: Bearer <token> required")
				return
			}
			p, err := validate(token, opts)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "AUTH_INVALID", err.Error())
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, p)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isExempt(path string, exemptions []string) bool {
	for _, p := range exemptions {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

type scopedClaims struct {
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

func validate(raw string, opts AuthOptions) (Principal, error) {
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods(allowedMethods(opts.Algorithm)),
		jwt.WithTimeFunc(opts.Now),
		jwt.WithExpirationRequired(),
	}
	if opts.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(opts.Audience))
	}
	parser := jwt.NewParser(parserOpts...)

	claims := &scopedClaims{}
	_, err := parser.ParseWithClaims(raw, claims, keyFunc(opts))
	if err != nil {
		return Principal{}, err
	}
	if claims.Subject == "" {
		return Principal{}, errors.New("sub claim missing")
	}
	return Principal{
		Subject: claims.Subject,
		Scope:   normaliseScope(claims.Scope),
	}, nil
}

func allowedMethods(alg string) []string {
	switch strings.ToUpper(alg) {
	case "RS256":
		return []string{"RS256"}
	default:
		return []string{"HS256"}
	}
}

func keyFunc(opts AuthOptions) jwt.Keyfunc {
	return func(_ *jwt.Token) (any, error) {
		if opts.SigningKey == "" {
			return nil, errors.New("signing key not configured")
		}
		switch strings.ToUpper(opts.Algorithm) {
		case "RS256":
			block, _ := pem.Decode([]byte(opts.SigningKey))
			if block == nil {
				return nil, errors.New("RS256 signing key is not PEM-encoded")
			}
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parse RSA public key: %w", err)
			}
			rsaPub, ok := pub.(*rsa.PublicKey)
			if !ok {
				return nil, errors.New("RS256 key is not an RSA public key")
			}
			return rsaPub, nil
		default:
			return []byte(opts.SigningKey), nil
		}
	}
}

func normaliseScope(s string) Scope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "write":
		return ScopeWrite
	case "admin":
		return ScopeAdmin
	default:
		return ScopeRead
	}
}

// IssueHS256 mints an HS256 JWT with the supplied claims. Used by the
// `/v1/auth/token` handler; tests use it to produce valid tokens.
func IssueHS256(secret, subject, audience string, scope Scope, ttl time.Duration, now time.Time) (string, time.Time, error) {
	if secret == "" {
		return "", time.Time{}, errors.New("signing key not configured")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	exp := now.Add(ttl)
	claims := scopedClaims{
		Scope: string(scope),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	if audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="rampart"`)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    code,
		"message": message,
	})
}
