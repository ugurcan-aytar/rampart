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
	require.Equal(t, 15*time.Second, c.SSEHeartbeatInterval)
	require.Equal(t, 256, c.SSESubscriberBuffer)
}
