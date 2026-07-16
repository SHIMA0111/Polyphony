package config

import (
	"testing"
	"time"
)

// withRequiredEnv sets the two required environment variables for the
// duration of the test and clears them afterwards. It also clears the
// optional DB_* duration variables so tests are isolated from any values
// inherited from the surrounding environment.
func withRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DB_MAX_CONN_LIFETIME", "")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "")
}

// TestLoadDBDurationDefaults verifies Load falls back to the documented
// default durations when no DB_* duration env vars are set.
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

// TestLoadDBDurationOverrides verifies Load applies DB_* duration env vars
// when they are set to valid, positive durations.
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

// TestLoadDBDurationInvalidFallsBackToDefault verifies Load falls back to the
// default DBMaxConnLifetime for values that fail to parse as a duration, and
// for durations that parse successfully but are not positive (zero or
// negative), since neither is a meaningful pool tuning value.
func TestLoadDBDurationInvalidFallsBackToDefault(t *testing.T) {
	tests := []string{"not-a-duration", "0s", "-1s"}

	for _, val := range tests {
		t.Run(val, func(t *testing.T) {
			withRequiredEnv(t)
			t.Setenv("DB_MAX_CONN_LIFETIME", val)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load should not fail on an invalid duration, got: %v", err)
			}
			if cfg.DBMaxConnLifetime != defaultDBMaxConnLifetime {
				t.Errorf("expected fallback to default DBMaxConnLifetime %v, got %v", defaultDBMaxConnLifetime, cfg.DBMaxConnLifetime)
			}
		})
	}
}

// TestLoadMissingRequiredVars verifies Load returns an error when the
// required DATABASE_URL and JWT_SECRET environment variables are unset.
func TestLoadMissingRequiredVars(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when DATABASE_URL and JWT_SECRET are unset")
	}
}
