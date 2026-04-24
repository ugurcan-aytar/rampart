package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/postgres"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/storagetest"
)

// sharedContainer is started once per test-package invocation and torn
// down after the last sub-test finishes. Each contract sub-test gets a
// fresh database (CREATE DATABASE rampart_<nonce>) so state never
// leaks between cases; booting a container per case costs 5-10 s each
// and multiplies by the number of contract cases.
type sharedContainer struct {
	container *tcpostgres.PostgresContainer
	adminDSN  string
	counter   int
}

func startSharedContainer(t *testing.T) *sharedContainer {
	t.Helper()
	if testing.Short() {
		t.Skip("postgres contract test needs docker — skipping in -short mode")
	}
	if os.Getenv("RAMPART_SKIP_POSTGRES_TESTS") != "" {
		t.Skip("RAMPART_SKIP_POSTGRES_TESTS is set")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("rampart_admin"),
		tcpostgres.WithUsername("rampart"),
		tcpostgres.WithPassword("rampart"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		t.Skipf("postgres container failed to start (docker unavailable?): %v", err)
	}
	// BasicWaitStrategies already covers pg_isready; the extra guard
	// here exists for flaky CI where the container reports ready
	// before accepting authenticated connections.
	if err := container.Start(ctx); err != nil {
		// Already running; that's fine.
		_ = err
	}
	wait.ForLog("database system is ready to accept connections")

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container DSN: %v", err)
	}
	return &sharedContainer{container: container, adminDSN: adminDSN}
}

func (c *sharedContainer) stop(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.container.Terminate(ctx); err != nil {
		t.Logf("container terminate: %v", err)
	}
}

// freshDSN creates a fresh database on the shared container and
// returns a DSN for it. Using template0 makes database creation
// ~20 ms rather than the ~1 s it takes to copy template1.
func (c *sharedContainer) freshDSN(t *testing.T) string {
	t.Helper()
	c.counter++
	dbName := fmt.Sprintf("rampart_contract_%d_%d", os.Getpid(), c.counter)

	ctx := context.Background()
	execCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, _, err := c.container.Exec(execCtx, []string{
		"psql", "-U", "rampart", "-d", "rampart_admin", "-c",
		fmt.Sprintf("CREATE DATABASE %s TEMPLATE template0;", dbName),
	})
	if err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}

	// Build the per-database DSN by swapping the path of the admin one.
	host, _ := c.container.Host(ctx)
	port, _ := c.container.MappedPort(ctx, "5432/tcp")
	return fmt.Sprintf(
		"postgres://rampart:rampart@%s:%s/%s?sslmode=disable",
		host, port.Port(), dbName,
	)
}

func TestPostgresContract(t *testing.T) {
	shared := startSharedContainer(t)
	t.Cleanup(func() { shared.stop(t) })

	storagetest.Run(t, func() storage.Storage {
		dsn := shared.freshDSN(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := postgres.MigrateUp(ctx, dsn); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		s, err := postgres.Open(ctx, dsn, 4)
		if err != nil {
			t.Fatalf("postgres open: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	})
}

func TestMigrateUpIdempotent(t *testing.T) {
	shared := startSharedContainer(t)
	t.Cleanup(func() { shared.stop(t) })

	dsn := shared.freshDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	// Second pass must be a no-op — operators running the engine
	// twice in a row should not see migration errors.
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatalf("second MigrateUp (should be idempotent): %v", err)
	}
}
