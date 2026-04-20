// Package config holds engine runtime settings. Phase 1 exposes sensible
// defaults; env/flag/YAML loading lands in Adım 4 alongside the CLI.
package config

// Config is the engine's runtime configuration.
type Config struct {
	HTTPAddr       string
	LogLevel       string
	TrustEngine    string
	ParserStrategy string
}

// Default returns the Phase 1 defaults.
func Default() *Config {
	return &Config{
		HTTPAddr:       ":8080",
		LogLevel:       "info",
		TrustEngine:    "always_trust",
		ParserStrategy: "go",
	}
}
