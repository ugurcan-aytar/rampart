package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/config"
)

func TestDefault(t *testing.T) {
	c := config.Default()
	require.Equal(t, ":8080", c.HTTPAddr)
	require.Equal(t, "info", c.LogLevel)
	require.Equal(t, "always_trust", c.TrustEngine)
	require.Equal(t, "go", c.ParserStrategy)
	require.Equal(t, "/var/run/rampart/native.sock", c.NativeSocketPath)
	require.Equal(t, 15*time.Second, c.SSEHeartbeatInterval)
	require.Equal(t, 256, c.SSESubscriberBuffer)
}

func TestFromEnv_NoOverrides(t *testing.T) {
	t.Setenv("RAMPART_PARSER_STRATEGY", "")
	t.Setenv("RAMPART_NATIVE_SOCKET", "")
	c := config.FromEnv()
	require.Equal(t, "go", c.ParserStrategy)
	require.Equal(t, "/var/run/rampart/native.sock", c.NativeSocketPath)
}

func TestFromEnv_StrategyOverride(t *testing.T) {
	t.Setenv("RAMPART_PARSER_STRATEGY", "native")
	t.Setenv("RAMPART_NATIVE_SOCKET", "/run/rampart/custom.sock")
	c := config.FromEnv()
	require.Equal(t, "native", c.ParserStrategy)
	require.Equal(t, "/run/rampart/custom.sock", c.NativeSocketPath)
}
