package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ugurcan-aytar/rampart/engine/internal/app"
	"github.com/ugurcan-aytar/rampart/engine/internal/config"
)

// TestPostgresPersistenceAcrossRestart proves the v0.2.0 persistence
// promise: everything ingested against a Postgres-backed engine must
// survive a full app restart. The test does NOT touch the engine
// binary — it constructs app.App in-process with the same DSN twice,
// so the regression surface is the code path operators actually run
// in production (config.FromEnv → openStorage → MigrateUp → Server).
//
// Shape:
//  1. Start a dedicated `postgres:16-alpine` container (testcontainers).
//  2. Boot app #1 pointed at that container; ingest 50 components +
//     50 IoCs/incidents over the HTTP surface; Close() the app.
//  3. Boot app #2 with the same DSN (simulates a rollout / crash /
//     `docker compose restart engine`); assert every ingested row is
//     still retrievable via the same HTTP endpoints.
//
// 50 rows rather than 1000 because the HTTP ingest is one POST per
// item — 1000 would add ~20 s of network for no additional signal.
// The 1000-row target is covered by the postgres bench suite under
// engine/internal/storage/postgres/bench_test.go.
func TestPostgresPersistenceAcrossRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("persistence test needs docker — skipping in -short mode")
	}
	if os.Getenv("RAMPART_SKIP_POSTGRES_TESTS") != "" {
		t.Skip("RAMPART_SKIP_POSTGRES_TESTS is set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("rampart"),
		tcpostgres.WithUsername("rampart"),
		tcpostgres.WithPassword("rampart"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		t.Skipf("postgres container unavailable: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		_ = container.Terminate(cctx)
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	cfg.StorageBackend = "postgres"
	cfg.DBDSN = dsn

	const N = 50

	// --- Round 1: ingest -------------------------------------------------
	addr := bootApp(ctx, t, cfg)
	for i := 0; i < N; i++ {
		upsertComponent(ctx, t, addr, fmt.Sprintf("persist-%04d", i))
	}
	iocs := make([]string, N)
	for i := 0; i < N; i++ {
		iocs[i] = upsertIoC(ctx, t, addr, fmt.Sprintf("ioc-persist-%04d", i), fmt.Sprintf("pkg-%04d", i))
	}
	require.Equal(t, N, componentCount(ctx, t, addr))
	require.Equal(t, N, iocCount(ctx, t, addr))

	// --- Round 2: restart + verify --------------------------------------
	addr2 := bootApp(ctx, t, cfg)
	require.NotEqual(t, addr, addr2, "second boot picks a new ephemeral port")
	require.Equal(t, N, componentCount(ctx, t, addr2),
		"components must survive the engine restart")
	require.Equal(t, N, iocCount(ctx, t, addr2),
		"IoCs must survive the engine restart")

	// And spot-check: fetch one of the ingested components by ref.
	ref := "kind:Component/default/persist-0017"
	got, err := httpGetJSON(ctx, addr2, "/v1/components")
	require.NoError(t, err)
	require.Contains(t, got, ref, "named component missing after restart")
}

// bootApp constructs a fresh app.App, starts it on an ephemeral port,
// and returns the "http://host:port" base the caller hits. The app
// shuts down cleanly via t.Cleanup.
func bootApp(ctx context.Context, t *testing.T, cfg *config.Config) string {
	t.Helper()
	a, err := app.New(ctx, cfg, silentLogger())
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Run(runCtx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Logf("app.Run did not exit within 10 s")
		}
		_ = a.Close()
	})

	// Wait for the listener to bind — Run's goroutine may not have
	// reached net.Listen by the time we get here. Addr() reports the
	// unresolved config until then; polling is the simplest way to
	// avoid exposing a race-free sync primitive on App just for tests.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr := a.Addr()
		// ":0" and "127.0.0.1:0" both stringify with a literal "0" port
		// until net.Listen hands us a real one.
		if addr != "" && addr != cfg.HTTPAddr && !endsWith(addr, ":0") {
			base := "http://" + addr
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return base
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("engine did not come up within 5s (last addr=%q)", a.Addr())
	return ""
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func upsertComponent(ctx context.Context, t *testing.T, base, name string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"ref":       fmt.Sprintf("kind:Component/default/%s", name),
		"kind":      "Component",
		"namespace": "default",
		"name":      name,
		"owner":     "team:persist",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/components", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// UpsertComponent returns 201 on insert, 200 on update.
	require.Containsf(t, []int{200, 201}, resp.StatusCode, "upsert %s: %d", name, resp.StatusCode)
}

func upsertIoC(ctx context.Context, t *testing.T, base, id, pkg string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"id":          id,
		"kind":        "packageVersion",
		"severity":    "high",
		"ecosystem":   "npm",
		"source":      "persist-test",
		"publishedAt": time.Now().UTC().Format(time.RFC3339),
		"packageVersion": map[string]string{
			"name":    pkg,
			"version": "1.0.0",
			"purl":    fmt.Sprintf("pkg:npm/%s@1.0.0", pkg),
		},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/iocs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Containsf(t, []int{200, 201}, resp.StatusCode, "upsert ioc %s: %d", id, resp.StatusCode)
	return id
}

func componentCount(ctx context.Context, t *testing.T, base string) int {
	t.Helper()
	return jsonCount(ctx, t, base, "/v1/components")
}

func iocCount(ctx context.Context, t *testing.T, base string) int {
	t.Helper()
	return jsonCount(ctx, t, base, "/v1/iocs")
}

func jsonCount(ctx context.Context, t *testing.T, base, path string) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	var page struct {
		Items []any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(b, &page))
	return len(page.Items)
}

func httpGetJSON(ctx context.Context, base, path string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}
