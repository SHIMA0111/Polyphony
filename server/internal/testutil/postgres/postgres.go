//go:build integration

// Package postgres provides a reusable testcontainers-backed PostgreSQL test
// harness for integration tests.
//
// It starts a disposable postgres:17-alpine container, applies every SQL
// migration under server/migrations/ (in filename order), and hands back a
// ready connection pool built via the shared database.NewPool helper. This
// lets integration tests exercise the real repository implementations
// (server/internal/interface/repository/postgres) against a real database,
// verifying concurrency and constraint behavior that in-memory mocks cannot.
//
// This package is gated behind the `integration` build tag: it pulls in
// Docker/testcontainers-go and requires a reachable Docker daemon, so it is
// intentionally excluded from ordinary `go test ./...` runs and only compiled
// when tests are run with `-tags=integration`.
package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
)

// Fixed test database credentials used for every container this helper
// starts. These never need to vary because each test gets its own isolated
// container.
const (
	testDBName     = "polyphony_test"
	testDBUser     = "polyphony_test"
	testDBPassword = "polyphony_test"
)

// New starts a disposable postgres:17-alpine testcontainer, applies every
// migration file under server/migrations/ (sorted by filename, so it keeps
// working as later steps add more migrations), and returns a *pgxpool.Pool
// connected to it via the shared database.NewPool helper.
//
// The container and the pool are both torn down automatically via
// t.Cleanup when the test (and any subtests) finish; callers must not call
// Close on the returned pool themselves. On any setup failure (container
// start, connection, or migration failure) New calls t.Fatalf and does not
// return.
func New(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase(testDBName),
		tcpostgres.WithUsername(testDBUser),
		tcpostgres.WithPassword(testDBPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("testutil/postgres: start container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("testutil/postgres: terminate container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testutil/postgres: build connection string: %v", err)
	}

	// Pool-tuning parameters mirror config.go's production defaults
	// (DB_MAX_CONN_LIFETIME/DB_MAX_CONN_IDLE_TIME/DB_HEALTH_CHECK_PERIOD);
	// there's no need for these to be configurable in tests.
	pool, err := database.NewPool(ctx, connStr, time.Hour, 30*time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("testutil/postgres: create connection pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := applyMigrations(ctx, pool); err != nil {
		t.Fatalf("testutil/postgres: apply migrations: %v", err)
	}

	return pool
}

// applyMigrations reads every *.sql file under server/migrations/ (excluding
// atlas.sum, which is not SQL), sorts them by filename (they are
// timestamp-prefixed, e.g. 20260220060722_initial.sql), and executes each
// file's full contents as a single batched statement using the simple query
// protocol, which allows multiple ;-separated statements per file.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// Use the simple query protocol so a file containing multiple
		// ;-separated statements executes as one batch; the extended
		// protocol pgxpool uses by default only allows a single statement.
		if _, err := pool.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}

	return nil
}

// migrationsDir returns the absolute path to server/migrations/, resolved
// relative to this source file's location on disk rather than the caller's
// working directory. This keeps the helper working regardless of which
// package (and therefore which `go test` working directory) invokes New.
func migrationsDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testutil/postgres: could not determine source file location")
	}
	// This file lives at server/internal/testutil/postgres/postgres.go;
	// server/migrations is three directories up from here.
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "migrations")
	return dir, nil
}
