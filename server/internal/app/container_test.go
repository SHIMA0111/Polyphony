package app

import (
	"context"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/config"
)

// TestNewContainerBadDatabaseURL verifies that NewContainer surfaces a
// connection error (rather than panicking or hanging) when given an
// unreachable/invalid database URL, without requiring a live PostgreSQL
// instance.
func TestNewContainerBadDatabaseURL(t *testing.T) {
	cfg := &config.Config{
		Port:                "8080",
		DatabaseURL:         "postgres://invalid-user:invalid-pass@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1",
		JWTSecret:           "test-secret",
		LLMGatewayURL:       "http://localhost:8081",
		CORSOrigins:         "http://localhost:3000",
		DBMaxConnLifetime:   time.Hour,
		DBMaxConnIdleTime:   30 * time.Minute,
		DBHealthCheckPeriod: time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	container, err := NewContainer(ctx, cfg)
	if err == nil {
		if container != nil && container.Pool != nil {
			container.Pool.Close()
		}
		t.Fatal("expected NewContainer to return an error for an unreachable database")
	}
	if container != nil {
		t.Fatalf("expected a nil container on error, got %+v", container)
	}
}
