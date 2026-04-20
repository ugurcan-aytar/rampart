// Package config holds engine runtime settings. Phase 1 exposes sensible
// defaults; env / flag / YAML loading lands in Adım 4 alongside the CLI.
package config

import (
	"os"
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
	return c
}
