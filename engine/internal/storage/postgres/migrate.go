// Package postgres is a production-grade Storage backend.
//
// Schema evolution goes through goose migrations under migrations/. The
// migration files are embedded into the binary so an operator running
// `engine` never needs the repo checked out to bring a fresh database
// up to the current version. MigrateUp is idempotent — re-running it
// against an already-current database is a no-op.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateUp applies any migrations the target database has not yet seen.
// The caller owns the pool; MigrateUp uses it only for the duration of
// the call, then the pool's connections are returned to the pool.
//
// pgx stdlib adapter is what goose requires — goose speaks database/sql,
// pgx does not by default, but pgx/stdlib registers a `pgx` driver that
// satisfies both.
func MigrateUp(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open for migrate: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)
	goose.SetTableName("rampart_schema_migrations")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("postgres: migrate up: %w", err)
	}
	return nil
}

// The unused import below keeps `pgx/stdlib` side-effects (driver
// registration) reachable even when a linter trims unused imports.
var _ = stdlib.GetDefaultDriver
