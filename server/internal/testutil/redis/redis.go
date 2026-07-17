// Package redis provides a reusable testcontainers-backed Redis test harness
// for integration tests, mirroring server/internal/testutil/postgres's
// pattern for the equivalent PostgreSQL harness.
//
// It starts a disposable redis:7-alpine container and hands back a ready
// *redis.Client built from its connection string. This lets integration
// tests (e.g. RedisHub) exercise real Redis Pub/Sub behavior rather than a
// mock.
//
// Unlike server/internal/testutil/postgres/postgres.go, this file is not
// itself gated behind the `integration` build tag: only its callers (test
// files under the `integration` tag, which pull in Docker/testcontainers-go
// transitively through this package) need a reachable Docker daemon. Keeping
// this helper tag-free lets `go mod tidy` see it as an ordinary dependency
// of the module, matching how testutil/postgres keeps
// github.com/testcontainers/testcontainers-go/modules/postgres a direct,
// non-indirect requirement in go.mod.
package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// New starts a disposable redis:7-alpine testcontainer and returns a
// *redis.Client connected to it.
//
// The container and the client are both torn down automatically via
// t.Cleanup when the test (and any subtests) finish; callers must not call
// Close on the returned client themselves. On any setup failure (container
// start or connection string parsing) New calls t.Fatalf and does not
// return.
func New(ctx context.Context, t *testing.T) *goredis.Client {
	t.Helper()
	return NewClient(ctx, t)
}

// NewConnString starts a disposable redis:7-alpine testcontainer (torn down
// automatically via t.Cleanup) and returns its connection string. Use this
// instead of New when a test needs more than one *redis.Client pointed at
// the same Redis instance — e.g. simulating multiple API server replicas
// that share one Redis deployment — since New's container is not otherwise
// reachable from outside the returned client.
func NewConnString(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("testutil/redis: start container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("testutil/redis: terminate container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("testutil/redis: get connection string: %v", err)
	}

	return connStr
}

// NewClient builds a *redis.Client connected to connStr's Redis instance. If
// connStr is empty, it first starts a fresh disposable container via
// NewConnString. The client is closed automatically via t.Cleanup.
//
// Passing the same connStr (from NewConnString) to multiple NewClient calls
// yields independent clients sharing one Redis instance, which is how tests
// simulate multiple API server replicas connected to the same Redis
// deployment.
func NewClient(ctx context.Context, t *testing.T, connStr ...string) *goredis.Client {
	t.Helper()

	cs := ""
	if len(connStr) > 0 {
		cs = connStr[0]
	}
	if cs == "" {
		cs = NewConnString(ctx, t)
	}

	opts, err := goredis.ParseURL(cs)
	if err != nil {
		t.Fatalf("testutil/redis: parse connection string: %v", err)
	}

	client := goredis.NewClient(opts)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("testutil/redis: close client: %v", err)
		}
	})

	return client
}
