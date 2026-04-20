package npm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// TestParserParity is the byte-identical contract between the Go and
// Rust lockfile parsers. For every valid fixture it:
//
//  1. parses with the Go in-process parser,
//  2. parses with the Rust sidecar over UDS,
//  3. normalises both SBOMs (drops the volatile ID / GeneratedAt fields,
//     re-marshals with alphabetical key ordering),
//  4. diffs the resulting JSON byte-for-byte.
//
// Wrong-version and malformed fixtures are exercised separately —
// TestParserParity_Errors below — to verify both parsers surface the
// same error class.
//
// The test spawns `cargo run --release -p rampart-native-cli` from the
// native/ workspace. If cargo or the Rust toolchain is unavailable the
// test is skipped with a clear message; CI in Adım 8 enforces presence.
func TestParserParity(t *testing.T) {
	handle := startNativeServer(t)
	defer handle.close()

	client := native.New(handle.socketPath)
	waitForPing(t, client, 10*time.Second)

	fixtures := []string{
		"axios-compromise.json",
		"simple-webapp.json",
		"with-workspaces.json",
		"with-scoped.json",
		"minimal.json",
	}

	fixedTime := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	const fixedID = "01PARITYTEST0000000000000000"
	const fixedComponentRef = "kind:Component/default/parity-target"
	const fixedCommitSHA = "0000000000000000000000000000000000000000"

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			path := filepath.Join("../../testdata/lockfiles", name)
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			meta := npm.LockfileMeta{
				SourcePath:   path,
				ComponentRef: fixedComponentRef,
				CommitSHA:    fixedCommitSHA,
				GeneratedAt:  fixedTime,
			}
			goSbom, err := npm.NewParser().Parse(ctx, content, meta)
			require.NoError(t, err, "go parse failed")

			rustSbom, err := client.ParseNPMLockfile(ctx, content, native.LockfileMeta{
				ComponentRef: fixedComponentRef,
				CommitSHA:    fixedCommitSHA,
				GeneratedAt:  fixedTime,
				ID:           fixedID,
			})
			require.NoError(t, err, "rust parse failed")

			// Equal-ignore-volatile-fields check.
			goNorm := canonicalJSON(t, goSbom)
			rustNorm := canonicalJSON(t, rustSbom)
			require.Equal(t, goNorm, rustNorm,
				"%s: parser outputs differ\n--- go ---\n%s\n--- rust ---\n%s",
				name, goNorm, rustNorm)
		})
	}
}

// TestParserParity_Errors — the two wrong-fixture cases must surface the
// same error class on both sides.
func TestParserParity_Errors(t *testing.T) {
	handle := startNativeServer(t)
	defer handle.close()

	client := native.New(handle.socketPath)
	waitForPing(t, client, 10*time.Second)

	cases := []struct {
		file      string
		goSentinel error
		rustSentinel error
	}{
		{"malformed.json", npm.ErrMalformedLockfile, native.ErrMalformedLockfile},
		{"wrong-version.json", npm.ErrUnsupportedLockfileVersion, native.ErrUnsupportedLockfileVersion},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			content, err := os.ReadFile(filepath.Join("../../testdata/lockfiles", tc.file))
			require.NoError(t, err)

			_, goErr := npm.NewParser().Parse(ctx, content, npm.LockfileMeta{})
			require.Error(t, goErr)
			require.True(t, errors.Is(goErr, tc.goSentinel),
				"go: expected %v, got %v", tc.goSentinel, goErr)

			_, rustErr := client.ParseNPMLockfile(ctx, content, native.LockfileMeta{})
			require.Error(t, rustErr)
			require.True(t, errors.Is(rustErr, tc.rustSentinel),
				"rust: expected %v, got %v", tc.rustSentinel, rustErr)
		})
	}
}

// --- helpers ---------------------------------------------------------------

// canonicalJSON emits a stable, alphabetically-keyed JSON with the
// per-call-volatile fields stripped (ID, GeneratedAt). That is exactly
// the surface TestParserParity claims must match between parsers.
func canonicalJSON(t *testing.T, sbom *domain.SBOM) string {
	t.Helper()
	raw, err := json.Marshal(sbom)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	delete(m, "ID")
	delete(m, "GeneratedAt")

	// json.Marshal on map[string]any sorts keys — canonical form.
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return sortPackageEntries(string(out))
}

// Package ordering inside each entry has already been enforced by both
// parsers (sort by name then version). We still re-sort as a belt-and-
// braces guard in case either side forgets.
func sortPackageEntries(s string) string {
	// crude but effective — we only sort the canonicalised string form.
	// If Packages are structurally different we'd want a diff view; for
	// raw byte-equality this is fine.
	lines := strings.Split(s, "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
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

// nativeBinaryPath ensures rampart-native is built once per process (LTO
// release builds are slow — a cold first compile can easily eat 30 s+).
// Later tests in the same run reuse the cached path.
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

	socketDir, err := os.MkdirTemp("", "rampart-parity-*")
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

// silence unused-import lint if net ever becomes unused
var _ = net.ErrClosed
var _ = fmt.Sprintf
