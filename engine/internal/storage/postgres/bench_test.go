package postgres_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/postgres"
)

// openBenchStore spins up a fresh Postgres database and returns a
// migrated Store. Sharing the container across benchmarks keeps the
// one-time pay-in to ~5 s; each bench gets an isolated schema so
// numbers don't carry row-count residue from a previous run.
func openBenchStore(b *testing.B, shared *sharedContainer) storage.Storage {
	b.Helper()
	dsn := shared.freshDSN(b)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		b.Fatalf("migrate up: %v", err)
	}
	s, err := postgres.Open(context.Background(), dsn, 8)
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = s.Close() })
	return s
}

// seedBench populates the store with `components` components and
// `sboms` SBOMs per component so hot-path reads operate against a
// realistic-ish fleet rather than empty tables.
func seedBench(b *testing.B, s storage.Storage, components, sbomsPerComponent int) {
	b.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < components; i++ {
		ref := fmt.Sprintf("kind:Component/default/bench-%04d", i)
		if err := s.UpsertComponent(ctx, domain.Component{
			Ref: ref, Kind: "Component", Namespace: "default",
			Name: fmt.Sprintf("bench-%04d", i), Owner: "team:bench",
		}); err != nil {
			b.Fatalf("seed component: %v", err)
		}
		for j := 0; j < sbomsPerComponent; j++ {
			sbom := domain.SBOM{
				ID:           fmt.Sprintf("sbom-%04d-%04d", i, j),
				ComponentRef: ref,
				Ecosystem:    "npm",
				GeneratedAt:  now.Add(-time.Duration(j) * time.Hour),
				Packages: []domain.PackageVersion{
					{Ecosystem: "npm", Name: "axios", Version: "1.11.0", PURL: "pkg:npm/axios@1.11.0"},
					{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", PURL: "pkg:npm/lodash@4.17.21"},
				},
			}
			if err := s.UpsertSBOM(ctx, sbom); err != nil {
				b.Fatalf("seed sbom: %v", err)
			}
		}
	}
}

// reportPercentiles attaches p50 / p95 / p99 to a benchmark's
// metrics. Go's default ns/op shows the mean; percentiles surface
// the tail-latency story operators actually care about.
func reportPercentiles(b *testing.B, name string, durations []time.Duration) {
	b.Helper()
	if len(durations) == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	idx := func(p float64) time.Duration {
		i := int(float64(len(durations)-1) * p)
		return durations[i]
	}
	b.ReportMetric(float64(idx(0.50).Microseconds()), name+"_p50_us")
	b.ReportMetric(float64(idx(0.95).Microseconds()), name+"_p95_us")
	b.ReportMetric(float64(idx(0.99).Microseconds()), name+"_p99_us")
}

func BenchmarkPostgres_UpsertSBOM(b *testing.B) {
	if testing.Short() {
		b.Skip("bench needs docker — skipping in -short mode")
	}
	shared := startSharedContainer(b)
	defer shared.stop(b)
	s := openBenchStore(b, shared)
	seedBench(b, s, 100, 1) // 100 components with 1 SBOM each

	ctx := context.Background()
	now := time.Now().UTC()
	durations := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		err := s.UpsertSBOM(ctx, domain.SBOM{
			ID:           fmt.Sprintf("bench-sbom-%08d", i),
			ComponentRef: fmt.Sprintf("kind:Component/default/bench-%04d", i%100),
			Ecosystem:    "npm",
			GeneratedAt:  now,
			Packages: []domain.PackageVersion{
				{Ecosystem: "npm", Name: "axios", Version: "1.11.0"},
			},
		})
		durations = append(durations, time.Since(start))
		if err != nil {
			b.Fatalf("upsert: %v", err)
		}
	}
	b.StopTimer()
	reportPercentiles(b, "upsert", durations)
}

func BenchmarkPostgres_ListSBOMsByComponent(b *testing.B) {
	if testing.Short() {
		b.Skip("bench needs docker — skipping in -short mode")
	}
	shared := startSharedContainer(b)
	defer shared.stop(b)
	s := openBenchStore(b, shared)
	seedBench(b, s, 100, 10) // 100 components × 10 SBOMs = 1000 rows

	ctx := context.Background()
	durations := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ref := fmt.Sprintf("kind:Component/default/bench-%04d", i%100)
		start := time.Now()
		got, err := s.ListSBOMsByComponent(ctx, ref)
		durations = append(durations, time.Since(start))
		if err != nil {
			b.Fatalf("list: %v", err)
		}
		if len(got) != 10 {
			b.Fatalf("want 10 sboms, got %d", len(got))
		}
	}
	b.StopTimer()
	reportPercentiles(b, "list", durations)
}

func BenchmarkPostgres_UpsertIncident(b *testing.B) {
	if testing.Short() {
		b.Skip("bench needs docker — skipping in -short mode")
	}
	shared := startSharedContainer(b)
	defer shared.stop(b)
	s := openBenchStore(b, shared)

	ctx := context.Background()
	now := time.Now().UTC()
	durations := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		err := s.UpsertIncident(ctx, domain.Incident{
			ID:                         fmt.Sprintf("bench-inc-%08d", i),
			IoCID:                      "ioc-bench",
			State:                      domain.StatePending,
			OpenedAt:                   now,
			LastTransitionedAt:         now,
			AffectedComponentsSnapshot: []string{"kind:Component/default/web", "kind:Component/default/api"},
		})
		durations = append(durations, time.Since(start))
		if err != nil {
			b.Fatalf("upsert: %v", err)
		}
	}
	b.StopTimer()
	reportPercentiles(b, "upsert", durations)
}

func BenchmarkPostgres_ListIncidents(b *testing.B) {
	if testing.Short() {
		b.Skip("bench needs docker — skipping in -short mode")
	}
	shared := startSharedContainer(b)
	defer shared.stop(b)
	s := openBenchStore(b, shared)

	ctx := context.Background()
	now := time.Now().UTC()
	// Seed 100 incidents — realistic "one week of low-severity
	// incidents" shape.
	for i := 0; i < 100; i++ {
		if err := s.UpsertIncident(ctx, domain.Incident{
			ID:                 fmt.Sprintf("seed-inc-%04d", i),
			IoCID:              "ioc-seed",
			State:              domain.StatePending,
			OpenedAt:           now,
			LastTransitionedAt: now,
		}); err != nil {
			b.Fatalf("seed incident: %v", err)
		}
	}

	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		got, err := s.ListIncidents(ctx)
		durations = append(durations, time.Since(start))
		if err != nil {
			b.Fatalf("list: %v", err)
		}
		if len(got) != 100 {
			b.Fatalf("want 100 incidents, got %d", len(got))
		}
	}
	b.StopTimer()
	reportPercentiles(b, "list", durations)
}
