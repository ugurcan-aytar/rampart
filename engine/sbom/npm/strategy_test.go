package npm_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// TestEffectiveStrategy_PassesThroughGo asserts the helper is a no-op
// when Go was requested (no probe, no network syscall).
func TestEffectiveStrategy_PassesThroughGo(t *testing.T) {
	got := npm.EffectiveStrategy(context.Background(), npm.StrategyGo, nil, nil)
	require.Equal(t, npm.StrategyGo, got)
}

// TestEffectiveStrategy_FallsBackWhenNativeUnavailable is ADR-0005's
// "Final Decision" exercised end-to-end: strategy=native requested,
// sidecar socket does not exist → helper drops to StrategyGo and
// writes a warn line so operators can see the fallback happened.
func TestEffectiveStrategy_FallsBackWhenNativeUnavailable(t *testing.T) {
	// A path inside t.TempDir() that we never bind — Dial fails fast.
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist.sock")
	client := native.New(nonexistent)

	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	got := npm.EffectiveStrategy(context.Background(), npm.StrategyNative, client, log)
	require.Equal(t, npm.StrategyGo, got, "unreachable native must fall back to go")
	require.Contains(t, logBuf.String(), "rampart-native unreachable",
		"fallback must be audited via a warn log — operators need the signal")
}

// TestEffectiveStrategy_NilClientFallsBack covers the "config asked
// for native but nobody wired the client" misconfiguration. Same
// outcome as an unreachable socket: log, drop to go.
func TestEffectiveStrategy_NilClientFallsBack(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, nil))

	got := npm.EffectiveStrategy(context.Background(), npm.StrategyNative, nil, log)
	require.Equal(t, npm.StrategyGo, got)
	require.Contains(t, logBuf.String(), "no native client wired")
}
