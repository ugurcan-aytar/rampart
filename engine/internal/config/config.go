// Package config holds engine runtime settings. Phase 1 exposes sensible
// defaults; env / flag / YAML loading lands in Adım 4 alongside the CLI.
package config

import (
	"os"
	"strings"
	"time"
)

// Config is the engine's runtime configuration.
type Config struct {
	HTTPAddr    string
	LogLevel    string
	TrustEngine string

	// ParserStrategy picks the npm lockfile backend. `"go"` (the default)
	// runs the in-process Go parser; `"native"` routes through the Rust
	// sidecar over UDS. ADR-0005 "Final Decision" records why `go` is
	// the current default — the Rust path is opt-in for deployments
	// that want parser sandboxing against malicious lockfiles.
	ParserStrategy string

	// NativeSocketPath is where the engine expects the Rust sidecar's
	// UDS socket. Only consulted when ParserStrategy == "native" (or
	// the engine boot path is probing before fallback). Container
	// default matches docker-compose.yml's shared volume.
	NativeSocketPath string

	// --- SSE (/v1/stream) --------------------------------------------------
	// Each deployment tunes these against proxy timeouts and traffic shape.

	// SSEHeartbeatInterval is how often the server emits a `: keep-alive`
	// comment to prevent intermediate proxies from closing the long-lived
	// connection. 15 s is below nginx default (60 s) and Cloudflare (100 s).
	SSEHeartbeatInterval time.Duration

	// SSESubscriberBuffer is the per-client channel capacity. Clients whose
	// buffer overflows are dropped (channel closed) — Publish never blocks.
	// 256 handles a burst of events from an IoC feed ingest without drops;
	// memory cost is ~64 KB per idle client.
	SSESubscriberBuffer int

	// --- Auth (/v1/*) ------------------------------------------------------
	// JWT validation on mutation routes. Disabled by default so v0.1.x
	// demos (`make demo-axios`) keep working; flip AuthEnabled=true in
	// production and supply a signing key via RAMPART_AUTH_SIGNING_KEY.

	// AuthEnabled gates the JWT middleware. false leaves the engine
	// unauthenticated — suitable for local / demo, never for production.
	AuthEnabled bool

	// AuthSigningKey is the HS256 shared secret (or RS256 PEM-encoded
	// public key, depending on AuthAlgorithm). Empty + AuthEnabled=true
	// is a boot-time error.
	AuthSigningKey string

	// AuthAlgorithm picks the JWT signing algorithm. `HS256` (the
	// default) treats AuthSigningKey as a shared secret; `RS256` treats
	// it as a PEM-encoded RSA public key.
	AuthAlgorithm string

	// AuthAudience, when non-empty, is asserted against the `aud`
	// claim. Left empty, the middleware skips the audience check.
	AuthAudience string

	// --- CORS (/v1/*) ------------------------------------------------------

	// CORSOrigins is the comma-delimited allow-list of origins permitted
	// to call the engine from a browser. Empty means "no browser
	// origins" — the demo/backstage flows stay unaffected because they
	// are server-to-server. Evaluated only when CORSAllowAll=false.
	CORSOrigins []string

	// CORSAllowAll, when true, echoes the request origin back as the
	// Access-Control-Allow-Origin header (effectively wildcard). Exists
	// for the v0.1.x demo path; production deployments should set an
	// explicit CORSOrigins allow-list instead.
	CORSAllowAll bool
}

// Default returns the Phase 1 defaults.
func Default() *Config {
	return &Config{
		HTTPAddr:             ":8080",
		LogLevel:             "info",
		TrustEngine:          "always_trust",
		ParserStrategy:       "go",
		NativeSocketPath:     "/var/run/rampart/native.sock",
		SSEHeartbeatInterval: 15 * time.Second,
		SSESubscriberBuffer:  256,
		AuthEnabled:          false,
		AuthAlgorithm:        "HS256",
		CORSAllowAll:         true,
	}
}

// FromEnv returns the Phase 1 defaults with the environment-variable
// overrides applied. Today it covers parser strategy and the native
// socket path — both are how `docker compose --profile native up`
// flips the engine into sidecar mode without rebuilding the image.
func FromEnv() *Config {
	c := Default()
	if v := os.Getenv("RAMPART_PARSER_STRATEGY"); v != "" {
		c.ParserStrategy = v
	}
	if v := os.Getenv("RAMPART_NATIVE_SOCKET"); v != "" {
		c.NativeSocketPath = v
	}
	if v := os.Getenv("RAMPART_AUTH_ENABLED"); v != "" {
		c.AuthEnabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("RAMPART_AUTH_SIGNING_KEY"); v != "" {
		c.AuthSigningKey = v
	}
	if v := os.Getenv("RAMPART_AUTH_ALGORITHM"); v != "" {
		c.AuthAlgorithm = v
	}
	if v := os.Getenv("RAMPART_AUTH_AUDIENCE"); v != "" {
		c.AuthAudience = v
	}
	if v, ok := os.LookupEnv("RAMPART_CORS_ORIGINS"); ok {
		c.CORSOrigins = splitAndTrim(v)
		c.CORSAllowAll = false
	}
	if v := os.Getenv("RAMPART_CORS_ALLOW_ALL"); v != "" {
		c.CORSAllowAll = strings.EqualFold(v, "true") || v == "1"
	}
	return c
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
