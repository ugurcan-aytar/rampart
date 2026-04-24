package middleware

import (
	"net/http"
	"strings"
)

// CORSOptions is the static configuration the CORS middleware needs.
//
// AllowAll echoes the request Origin back as the allow-origin header
// unconditionally — fine for local demos / `make demo-axios`, never
// for production. When AllowAll is false, Origins is the allow-list:
// requests from an origin not in the list get no allow-origin header
// and the browser blocks the response.
//
// AllowedMethods / AllowedHeaders / ExposedHeaders have engine-sane
// defaults; callers rarely need to override them.
type CORSOptions struct {
	AllowAll       bool
	Origins        []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
}

// DefaultCORSOptions returns the permissive config that ships with the
// engine before an operator narrows it via RAMPART_CORS_ORIGINS. It
// matches the v0.1.x inline behaviour so existing demo flows keep
// working unchanged.
func DefaultCORSOptions() CORSOptions {
	return CORSOptions{
		AllowAll:       true,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Last-Event-ID"},
		ExposedHeaders: []string{"Content-Type"},
	}
}

// CORS returns the middleware. When the origin is not permitted, the
// middleware still forwards the request to the next handler — it just
// omits the `Access-Control-Allow-Origin` header so the browser
// enforces the block. Server-to-server callers (CLI / CI / the
// Backstage backend proxy) do not care about CORS headers and are
// unaffected.
func CORS(opts CORSOptions) func(http.Handler) http.Handler {
	methods := strings.Join(defaultedSlice(opts.AllowedMethods, []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}), ", ")
	headers := strings.Join(defaultedSlice(opts.AllowedHeaders, []string{"Authorization", "Content-Type", "Last-Event-ID"}), ", ")
	exposed := strings.Join(defaultedSlice(opts.ExposedHeaders, []string{"Content-Type"}), ", ")

	allowed := make(map[string]struct{}, len(opts.Origins))
	for _, o := range opts.Origins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowOrigin(origin, opts.AllowAll, allowed) {
				w.Header().Set("Access-Control-Allow-Origin", echoOrWildcard(origin, opts.AllowAll))
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", methods)
				w.Header().Set("Access-Control-Allow-Headers", headers)
				w.Header().Set("Access-Control-Expose-Headers", exposed)
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func allowOrigin(origin string, allowAll bool, allowed map[string]struct{}) bool {
	if allowAll {
		return true
	}
	if origin == "" {
		return false
	}
	_, ok := allowed[origin]
	return ok
}

func echoOrWildcard(origin string, allowAll bool) string {
	if allowAll && origin == "" {
		return "*"
	}
	return origin
}

func defaultedSlice(s, fallback []string) []string {
	if len(s) == 0 {
		return fallback
	}
	return s
}
