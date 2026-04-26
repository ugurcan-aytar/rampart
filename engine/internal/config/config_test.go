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
	require.Equal(t, "postgres", c.StorageBackend)
	require.Equal(t, int32(10), c.DBMaxConns)
}

func TestFromEnv_StorageOverrides(t *testing.T) {
	t.Setenv("RAMPART_STORAGE", "memory")
	t.Setenv("RAMPART_DB_DSN", "postgres://rampart:rampart@db:5432/rampart?sslmode=disable")
	t.Setenv("RAMPART_DB_MAX_CONNS", "25")
	c := config.FromEnv()
	require.Equal(t, "memory", c.StorageBackend)
	require.Equal(t, "postgres://rampart:rampart@db:5432/rampart?sslmode=disable", c.DBDSN)
	require.Equal(t, int32(25), c.DBMaxConns)
}

func TestFromEnv_InvalidMaxConnsKeepsDefault(t *testing.T) {
	t.Setenv("RAMPART_DB_MAX_CONNS", "not-a-number")
	c := config.FromEnv()
	require.Equal(t, int32(10), c.DBMaxConns, "unparseable max-conns must keep the default")
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

func TestDefault_AuthDisabled(t *testing.T) {
	c := config.Default()
	require.False(t, c.AuthEnabled, "auth must be off by default for v0.1.x backward-compat")
	require.Equal(t, "HS256", c.AuthAlgorithm)
	require.True(t, c.CORSAllowAll, "CORS wildcard stays on by default until operator narrows it")
	require.Empty(t, c.CORSOrigins)
}

func TestFromEnv_AuthOverrides(t *testing.T) {
	t.Setenv("RAMPART_AUTH_ENABLED", "true")
	t.Setenv("RAMPART_AUTH_SIGNING_KEY", "s3cr3t")
	t.Setenv("RAMPART_AUTH_ALGORITHM", "RS256")
	t.Setenv("RAMPART_AUTH_AUDIENCE", "rampart-prod")
	c := config.FromEnv()
	require.True(t, c.AuthEnabled)
	require.Equal(t, "s3cr3t", c.AuthSigningKey)
	require.Equal(t, "RS256", c.AuthAlgorithm)
	require.Equal(t, "rampart-prod", c.AuthAudience)
}

func TestFromEnv_CORSOriginsOverride(t *testing.T) {
	t.Setenv("RAMPART_CORS_ORIGINS", "https://app.example.com, https://backstage.example.com")
	c := config.FromEnv()
	require.Equal(t, []string{"https://app.example.com", "https://backstage.example.com"}, c.CORSOrigins)
	require.False(t, c.CORSAllowAll, "explicit origin list must turn off the wildcard")
}

func TestFromEnv_CORSAllowAll(t *testing.T) {
	t.Setenv("RAMPART_CORS_ALLOW_ALL", "false")
	c := config.FromEnv()
	require.False(t, c.CORSAllowAll)
}
