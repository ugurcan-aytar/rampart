package gomod_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
	"github.com/ugurcan-aytar/rampart/engine/sbom/gomod"
)

// TestParserParity diffs Go and Rust gomod parser outputs byte-for-byte.
// Mirrors engine/sbom/npm/parity_test.go but reads (go.sum, go.mod) pairs
// from per-fixture subdirectories under engine/testdata/lockfiles/gomod/.
func TestParserParity(t *testing.T) {
	handle := startNativeServer(t)
	defer handle.close()

	client := native.New(handle.socketPath)
	waitForPing(t, client, 10*time.Second)

	fixtures := []string{
		"simple",
		"with-replace",
		"medium",
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			fixDir := filepath.Join("../../testdata/lockfiles/gomod", name)
			gosum, err := os.ReadFile(filepath.Join(fixDir, "go.sum"))
			require.NoError(t, err, "read go.sum")
			gomodContent, err := os.ReadFile(filepath.Join(fixDir, "go.mod"))
			require.NoError(t, err, "read go.mod")

			goParsed, err := gomod.NewParser().Parse(ctx, gosum, gomodContent)
			require.NoError(t, err, "go parse failed")

			rustParsed, err := client.ParseGoModule(ctx, gosum, gomodContent)
			require.NoError(t, err, "rust parse failed")

			goBytes := mustMarshal(t, goParsed)
			rustBytes := mustMarshal(t, rustParsed)
			if !bytes.Equal(goBytes, rustBytes) {
				t.Fatalf("%s: parser outputs differ\n--- go ---\n%s\n--- rust ---\n%s",
					name, goBytes, rustBytes)
			}
		})
	}
}

// TestParserParity_Errors — both sides surface the same error class for a
// malformed go.sum.
func TestParserParity_Errors(t *testing.T) {
	handle := startNativeServer(t)
	defer handle.close()

	client := native.New(handle.socketPath)
	waitForPing(t, client, 10*time.Second)

	malformed := []byte("github.com/foo/bar v1.0.0\n") // missing hash

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, goErr := gomod.NewParser().Parse(ctx, malformed, nil)
	require.Error(t, goErr)
	require.True(t, errors.Is(goErr, gomod.ErrMalformedLockfile),
		"go: expected ErrMalformedLockfile, got %v", goErr)

	_, rustErr := client.ParseGoModule(ctx, malformed, nil)
	require.Error(t, rustErr)
	require.True(t, errors.Is(rustErr, native.ErrMalformedLockfile),
		"rust: expected ErrMalformedLockfile, got %v", rustErr)
}

// --- helpers (mirror npm/parity_test.go; intentional duplication so each
// parity test file is self-contained and removable in isolation) ---------

func mustMarshal(t *testing.T, p *domain.ParsedSBOM) []byte {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return raw
}

type serverHandle struct {
	cmd        *exec.Cmd
	socketPath string
	socketDir  string
}

func (h *serverHandle) close() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = h.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = h.cmd.Process.Kill()
		}
	}
	if h.socketDir != "" {
		_ = os.RemoveAll(h.socketDir)
	}
}

var nativeBinaryCache = struct {
	sync.Once
	path string
	err  error
}{}

func ensureNativeBinary(t *testing.T) string {
	t.Helper()
	nativeBinaryCache.Do(func() {
		nativeDir, err := filepath.Abs("../../../native")
		if err != nil {
			nativeBinaryCache.err = err
			return
		}
		build := exec.Command("cargo", "build", "--release",
			"--manifest-path", filepath.Join(nativeDir, "Cargo.toml"),
			"--bin", "rampart-native",
			"--quiet")
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			nativeBinaryCache.err = fmt.Errorf("cargo build rampart-native: %w", err)
			return
		}
		nativeBinaryCache.path = filepath.Join(nativeDir, "target", "release", "rampart-native")
	})
	if nativeBinaryCache.err != nil {
		t.Fatalf("build rampart-native failed: %v", nativeBinaryCache.err)
	}
	return nativeBinaryCache.path
}

func startNativeServer(t *testing.T) *serverHandle {
	t.Helper()
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skipf("cargo not found in PATH — skipping Rust parity test (%v)", err)
	}
	binaryPath := ensureNativeBinary(t)

	socketDir, err := os.MkdirTemp("", "rampart-parity-gomod-*")
	require.NoError(t, err)
	socketPath := filepath.Join(socketDir, "native.sock")

	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(),
		"RAMPART_NATIVE_SOCKET="+socketPath,
		"RUST_LOG=warn",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	t.Logf("spawned rampart-native pid=%d socket=%s", cmd.Process.Pid, socketPath)
	return &serverHandle{cmd: cmd, socketPath: socketPath, socketDir: socketDir}
}

func waitForPing(t *testing.T, client *native.Client, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := client.Ping(ctx)
		cancel()
		if err == nil {
			return
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("rampart-native never answered ping within %s: %v", timeout, lastErr)
}
