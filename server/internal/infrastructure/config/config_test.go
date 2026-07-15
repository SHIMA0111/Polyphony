package config

import (
	"testing"
	"time"
)

// withRequiredEnv sets the two required environment variables for the
// duration of the test and clears them afterwards.
func withRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "test-secret")
}

func TestLoadDBDurationDefaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBMaxConnLifetime != defaultDBMaxConnLifetime {
		t.Errorf("expected default DBMaxConnLifetime %v, got %v", defaultDBMaxConnLifetime, cfg.DBMaxConnLifetime)
	}
	if cfg.DBMaxConnIdleTime != defaultDBMaxConnIdleTime {
		t.Errorf("expected default DBMaxConnIdleTime %v, got %v", defaultDBMaxConnIdleTime, cfg.DBMaxConnIdleTime)
	}
	if cfg.DBHealthCheckPeriod != defaultDBHealthCheckPeriod {
		t.Errorf("expected default DBHealthCheckPeriod %v, got %v", defaultDBHealthCheckPeriod, cfg.DBHealthCheckPeriod)
	}
}

func TestLoadDBDurationOverrides(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "15m")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBMaxConnLifetime != 2*time.Hour {
		t.Errorf("expected DBMaxConnLifetime 2h, got %v", cfg.DBMaxConnLifetime)
	}
	if cfg.DBMaxConnIdleTime != 15*time.Minute {
		t.Errorf("expected DBMaxConnIdleTime 15m, got %v", cfg.DBMaxConnIdleTime)
	}
	if cfg.DBHealthCheckPeriod != 30*time.Second {
		t.Errorf("expected DBHealthCheckPeriod 30s, got %v", cfg.DBHealthCheckPeriod)
	}
}

func TestLoadDBDurationInvalidFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("DB_MAX_CONN_LIFETIME", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on an invalid duration, got: %v", err)
	}
	if cfg.DBMaxConnLifetime != defaultDBMaxConnLifetime {
		t.Errorf("expected fallback to default DBMaxConnLifetime %v, got %v", defaultDBMaxConnLifetime, cfg.DBMaxConnLifetime)
	}
}

func TestLoadMissingRequiredVars(t *testing.T) {
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when DATABASE_URL and JWT_SECRET are unset")
	}
}

func TestLoadS3Defaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.S3Endpoint != "http://localhost:9000" {
		t.Errorf("expected default S3Endpoint http://localhost:9000, got %v", cfg.S3Endpoint)
	}
	if cfg.S3Region != "us-east-1" {
		t.Errorf("expected default S3Region us-east-1, got %v", cfg.S3Region)
	}
	if cfg.S3Bucket != "polyphony-attachments" {
		t.Errorf("expected default S3Bucket polyphony-attachments, got %v", cfg.S3Bucket)
	}
	if cfg.S3AccessKey != "minioadmin" {
		t.Errorf("expected default S3AccessKey minioadmin, got %v", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "minioadmin" {
		t.Errorf("expected default S3SecretKey minioadmin, got %v", cfg.S3SecretKey)
	}
	if !cfg.S3ForcePathStyle {
		t.Errorf("expected default S3ForcePathStyle true, got %v", cfg.S3ForcePathStyle)
	}
}

func TestLoadS3Overrides(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("S3_ENDPOINT", "http://minio.example.com:9000")
	t.Setenv("S3_REGION", "eu-west-1")
	t.Setenv("S3_BUCKET", "custom-bucket")
	t.Setenv("S3_ACCESS_KEY", "custom-access-key")
	t.Setenv("S3_SECRET_KEY", "custom-secret-key")
	t.Setenv("S3_FORCE_PATH_STYLE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.S3Endpoint != "http://minio.example.com:9000" {
		t.Errorf("expected overridden S3Endpoint, got %v", cfg.S3Endpoint)
	}
	if cfg.S3Region != "eu-west-1" {
		t.Errorf("expected overridden S3Region, got %v", cfg.S3Region)
	}
	if cfg.S3Bucket != "custom-bucket" {
		t.Errorf("expected overridden S3Bucket, got %v", cfg.S3Bucket)
	}
	if cfg.S3AccessKey != "custom-access-key" {
		t.Errorf("expected overridden S3AccessKey, got %v", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "custom-secret-key" {
		t.Errorf("expected overridden S3SecretKey, got %v", cfg.S3SecretKey)
	}
	if cfg.S3ForcePathStyle {
		t.Errorf("expected overridden S3ForcePathStyle false, got %v", cfg.S3ForcePathStyle)
	}
}
