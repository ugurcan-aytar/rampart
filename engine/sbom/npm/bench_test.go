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
	meta := npm.LockfileMeta{SourcePath: name}
	ctx := context.Background()
	b.ResetTimer()
	b.SetBytes(int64(len(content)))
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(ctx, content, meta); err != nil {
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
	meta := native.LockfileMeta{}
	ctx = context.Background()
	b.ResetTimer()
	b.SetBytes(int64(len(content)))
	for i := 0; i < b.N; i++ {
		if _, err := client.ParseNPMLockfile(ctx, content, meta); err != nil {
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

// BenchmarkParseGo_HugeMonorepo and BenchmarkParseNative_HugeMonorepo
// need a large generated fixture. Adım 8 wires `make generate-large-fixture`
// to write it to testdata/lockfiles/huge-monorepo.json on demand (50 MiB+
// — not committed). Until then the bench auto-skips.
func BenchmarkParseGo_HugeMonorepo(b *testing.B)     { skipIfMissing(b, "huge-monorepo.json", benchmarkGo) }
func BenchmarkParseNative_HugeMonorepo(b *testing.B) { skipIfMissing(b, "huge-monorepo.json", benchmarkNative) }

func skipIfMissing(b *testing.B, name string, run func(*testing.B, string)) {
	p := filepath.Join("../../testdata/lockfiles", name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		b.Skipf("%s not generated — run `make generate-large-fixture` (Adım 8)", name)
	}
	run(b, name)
}
