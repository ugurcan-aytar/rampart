package npm_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// Benchmarks are deliberately honest: small lockfiles lose against the
// Go parser (IPC overhead dominates), huge lockfiles pay back the cost.
// Results written into docs/benchmarks/sbom-parser.md.
//
// The `Native` benchmarks are skipped unless a rampart-native server is
// already listening at RAMPART_NATIVE_SOCKET — the intent is that CI
// (Adım 8 parity workflow) starts the server once and runs both
// benchmark sets against it.

func loadFixture(b *testing.B, name string) []byte {
	b.Helper()
	body, err := os.ReadFile(filepath.Join("../../testdata/lockfiles", name))
	if err != nil {
		b.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func benchmarkGo(b *testing.B, name string) {
	content := loadFixture(b, name)
	parser := npm.NewParser()
	ctx := context.Background()
	b.ResetTimer()
	b.SetBytes(int64(len(content)))
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(ctx, content); err != nil {
			b.Fatalf("parse: %v", err)
		}
	}
}

func benchmarkNative(b *testing.B, name string) {
	socket := os.Getenv("RAMPART_NATIVE_SOCKET")
	if socket == "" {
		b.Skip("RAMPART_NATIVE_SOCKET unset — start rampart-native before running native benchmarks")
	}
	client := native.New(socket)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		b.Skipf("rampart-native not reachable at %s: %v", socket, err)
	}
	content := loadFixture(b, name)
	ctx = context.Background()
	b.ResetTimer()
	b.SetBytes(int64(len(content)))
	for i := 0; i < b.N; i++ {
		if _, err := client.ParseNPMLockfile(ctx, content); err != nil {
			b.Fatalf("parse: %v", err)
		}
	}
}

// Small (<1 KiB): the compromise fixture. Go wins here by orders of
// magnitude — every native call is 1 UDS connect + 1 JSON round-trip.
func BenchmarkParseGo_AxiosCompromise(b *testing.B)     { benchmarkGo(b, "axios-compromise.json") }
func BenchmarkParseNative_AxiosCompromise(b *testing.B) { benchmarkNative(b, "axios-compromise.json") }

// Medium (~2 KiB): simple-webapp.
func BenchmarkParseGo_SimpleWebapp(b *testing.B)     { benchmarkGo(b, "simple-webapp.json") }
func BenchmarkParseNative_SimpleWebapp(b *testing.B) { benchmarkNative(b, "simple-webapp.json") }

// Scoped packages — exercises URL encoding path.
func BenchmarkParseGo_WithScoped(b *testing.B)     { benchmarkGo(b, "with-scoped.json") }
func BenchmarkParseNative_WithScoped(b *testing.B) { benchmarkNative(b, "with-scoped.json") }

// Generated fixtures — populated by `go run ./scripts/gen-lockfile-fixture`.
// Not committed to the repo (sizes grow into tens of MiB); see
// `docs/benchmarks/sbom-parser.md` for reproduction steps. Each bench
// auto-skips when the fixture is absent so the same file runs cleanly
// on developer laptops that haven't generated them yet.
func BenchmarkParseGo_Medium200Pkgs(b *testing.B) {
	skipIfMissing(b, "medium-200-pkgs.json", benchmarkGo)
}
func BenchmarkParseNative_Medium200Pkgs(b *testing.B) {
	skipIfMissing(b, "medium-200-pkgs.json", benchmarkNative)
}

func BenchmarkParseGo_Medium2kPkgs(b *testing.B) {
	skipIfMissing(b, "medium-2k-pkgs.json", benchmarkGo)
}
func BenchmarkParseNative_Medium2kPkgs(b *testing.B) {
	skipIfMissing(b, "medium-2k-pkgs.json", benchmarkNative)
}

func BenchmarkParseGo_Large20kPkgs(b *testing.B) {
	skipIfMissing(b, "large-20k-pkgs.json", benchmarkGo)
}
func BenchmarkParseNative_Large20kPkgs(b *testing.B) {
	skipIfMissing(b, "large-20k-pkgs.json", benchmarkNative)
}

func BenchmarkParseGo_Huge100kPkgs(b *testing.B) {
	skipIfMissing(b, "huge-100k-pkgs.json", benchmarkGo)
}
func BenchmarkParseNative_Huge100kPkgs(b *testing.B) {
	skipIfMissing(b, "huge-100k-pkgs.json", benchmarkNative)
}

func skipIfMissing(b *testing.B, name string, run func(*testing.B, string)) {
	p := filepath.Join("../../testdata/lockfiles", name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		b.Skipf("%s not generated — run `go run ./cmd/gen-lockfile-fixture -packages N -output testdata/lockfiles/%s` from engine/", name, name)
	}
	run(b, name)
}
