package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/app"
	"github.com/ugurcan-aytar/rampart/engine/internal/config"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// captureStdout swaps os.Stdout with a pipe, runs fn, and returns whatever fn wrote.
func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()
	orig := os.Stdout
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(copyDone)
	}()

	runErr := fn()
	os.Stdout = orig
	_ = w.Close()
	<-copyDone
	_ = r.Close()
	return buf.Bytes(), runErr
}

func TestMain_ParseSBOMSubcommand_Axios(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return app.Main(context.Background(), []string{"parse-sbom", "../../testdata/lockfiles/axios-compromise.json"})
	})
	require.NoError(t, err)

	var sbom domain.SBOM
	require.NoError(t, json.Unmarshal(out, &sbom))
	require.Equal(t, "npm", sbom.Ecosystem)
	require.Equal(t, "npm-package-lock-v3", sbom.SourceFormat)

	byName := map[string]domain.PackageVersion{}
	for _, p := range sbom.Packages {
		byName[p.Name] = p
	}
	require.Contains(t, byName, "axios")
	require.Equal(t, "1.11.0", byName["axios"].Version)
	require.Contains(t, byName, "plain-crypto-js")
	require.Equal(t, "4.2.1", byName["plain-crypto-js"].Version)
}

func TestMain_ParseSBOM_WithFlags(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return app.Main(context.Background(), []string{
			"parse-sbom",
			"--component-ref", "component:default/web-app",
			"--commit-sha", "abc123deadbeef",
			"../../testdata/lockfiles/axios-compromise.json",
		})
	})
	require.NoError(t, err)

	var sbom domain.SBOM
	require.NoError(t, json.Unmarshal(out, &sbom))
	require.Equal(t, "component:default/web-app", sbom.ComponentRef, "flag value must land on SBOM")
	require.Equal(t, "abc123deadbeef", sbom.CommitSHA)
}

func TestMain_ParseSBOM_UnknownFlag(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom", "--nope", "x", "some.json"})
	require.Error(t, err)
}

func TestMain_ParseSBOM_MissingArg(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom"})
	require.Error(t, err)
}

func TestMain_ParseSBOM_MissingFile(t *testing.T) {
	err := app.Main(context.Background(), []string{"parse-sbom", "/definitely/does/not/exist.json"})
	require.Error(t, err)
}

// --- App.New + App.Run lifecycle -----------------------------------------
//
// The parse-sbom tests above exercise the subcommand dispatch inside Main.
// These tests cover the server path: initialization (New) and lifecycle
// (Run + graceful shutdown on context cancel) — which are two distinct
// concerns and need to be verified separately.

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestApp_New_WithDefaults(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := app.New(context.Background(), cfg, silentLogger())
	require.NoError(t, err)
	require.NotNil(t, a)
	require.NoError(t, a.Close())
}

func TestApp_New_NilLoggerFallsBack(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := app.New(context.Background(), cfg, nil)
	require.NoError(t, err, "nil logger must fall back to slog.Default()")
	require.NotNil(t, a)
	require.NoError(t, a.Close())
}

func TestApp_Run_GracefulShutdownOnCancel(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := app.New(context.Background(), cfg, silentLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- a.Run(ctx) }()

	// Give the server a tick to bind before asking for shutdown.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "Run must return nil on graceful shutdown")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit within 5s of context cancel")
	}
}

func TestApp_Close_Idempotent(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := app.New(context.Background(), cfg, silentLogger())
	require.NoError(t, err)
	require.NoError(t, a.Close())
	require.NoError(t, a.Close(), "Close must be safe to call twice (memory backend is nil-op)")
}
