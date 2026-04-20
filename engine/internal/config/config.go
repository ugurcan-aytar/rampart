// Package config holds engine runtime settings. Phase 1 exposes sensible
// defaults; env / flag / YAML loading lands in Adım 4 alongside the CLI.
package config

import "time"

// Config is the engine's runtime configuration.
type Config struct {
	HTTPAddr       string
	LogLevel       string
	TrustEngine    string
	ParserStrategy string

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
		SSEHeartbeatInterval: 15 * time.Second,
		SSESubscriberBuffer:  256,
	}
}
