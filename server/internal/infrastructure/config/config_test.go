package config

import (
	"testing"
	"time"
)

// withRequiredEnv sets the two required environment variables and clears
// every optional environment variable Load reads (via t.Setenv("...", ""),
// which os.Getenv cannot distinguish from unset, and which t.Setenv restores
// to its prior value after the test regardless).
//
// Without this, a test asserting a default value (e.g.
// TestLoadRateLimitAndWhoamiCacheDefaults) would silently pass or fail based
// on whatever happened to already be set in the ambient shell/CI
// environment (e.g. a developer's .env sourced into their shell, or
// leftover exported vars from a previous docker compose run) rather than
// proving Load()'s own default-selection logic. This list must be kept in
// sync with every os.Getenv("...") call (including the ones behind
// parseDurationEnv/parseIntEnv) in config.go.
func withRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "test-secret")

	optionalEnvVars := []string{
		"PORT",
		"LLM_GATEWAY_URL",
		"CORS_ORIGINS",
		"DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME",
		"DB_HEALTH_CHECK_PERIOD",
		"S3_ENDPOINT",
		"S3_REGION",
		"S3_BUCKET",
		"S3_ACCESS_KEY",
		"S3_SECRET_KEY",
		"S3_FORCE_PATH_STYLE",
		"WS_TICKET_SECRET",
		"AUTH_MODE",
		"KRATOS_PUBLIC_URL",
		"KRATOS_ADMIN_URL",
		"KRATOS_COOKIE_NAME",
		"LLM_GATEWAY_TRANSPORT",
		"LLM_GATEWAY_GRPC_ADDR",
		"LLM_GATEWAY_GRPC_MAX_RETRIES",
		"LLM_GATEWAY_GRPC_BASE_BACKOFF",
		"REDIS_URL",
		"MESSAGE_HUB_DRIVER",
		"DEFAULT_AI_MODEL",
		"RATE_LIMIT_LOGIN_PER_MINUTE",
		"RATE_LIMIT_AI_INVOKE_PER_MINUTE",
		"WHOAMI_CACHE_TTL",
		"STRIPE_SECRET_KEY",
		"STRIPE_WEBHOOK_SECRET",
		"STRIPE_PLANS_JSON",
		"STRIPE_TOKEN_PACKAGES_JSON",
		"STRIPE_CHECKOUT_SUCCESS_URL",
		"STRIPE_CHECKOUT_CANCEL_URL",
	}
	for _, name := range optionalEnvVars {
		t.Setenv(name, "")
	}
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

// TestLoadCORSOriginsWildcardReturnsError proves that Load fails fast when
// CORS_ORIGINS contains a bare "*", since app/router.go pairs
// AllowCredentials: true with CORSOrigins on the documented assumption that
// it never contains a wildcard.
func TestLoadCORSOriginsWildcardReturnsError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("CORS_ORIGINS", "*")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when CORS_ORIGINS is a bare wildcard")
	}
}

// TestLoadCORSOriginsWildcardInListReturnsError proves the same fail-fast
// applies when "*" appears alongside other explicit origins in the
// comma-separated list, not just as the sole value.
func TestLoadCORSOriginsWildcardInListReturnsError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("CORS_ORIGINS", "http://localhost:3000, * ,https://example.com")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when CORS_ORIGINS' list includes a wildcard entry")
	}
}

// TestLoadCORSOriginsExplicitListSucceeds proves that a comma-separated list
// of explicit (non-wildcard) origins is still accepted.
func TestLoadCORSOriginsExplicitListSucceeds(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("CORS_ORIGINS", "http://localhost:3000,https://example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.CORSOrigins != "http://localhost:3000,https://example.com" {
		t.Errorf("unexpected CORSOrigins: %v", cfg.CORSOrigins)
	}
}

// TestLoadCORSOriginsTrimsWhitespaceAndDropsEmpties proves that Load trims
// leading/trailing whitespace from each comma-separated CORS_ORIGINS entry
// and drops empty entries (e.g. from a trailing comma), so an
// operator-supplied value with stray spaces still parses to the exact
// origins a browser's Origin header would present.
func TestLoadCORSOriginsTrimsWhitespaceAndDropsEmpties(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("CORS_ORIGINS", " https://a.com , https://b.com ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.CORSOrigins != "https://a.com,https://b.com" {
		t.Errorf("expected trimmed CORSOrigins %q, got %q", "https://a.com,https://b.com", cfg.CORSOrigins)
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

// TestLoadS3ForcePathStyleInvalidValueDefaultsWithoutError proves that an
// unparseable S3_FORCE_PATH_STYLE value doesn't fail Load: it's an optional
// tuning knob (like DB_MAX_CONN_LIFETIME etc.), so an invalid value just
// falls back to the default (true) with a logged warning instead of a
// startup error.
func TestLoadS3ForcePathStyleInvalidValueDefaultsWithoutError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("S3_FORCE_PATH_STYLE", "not-a-bool")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed with an invalid S3_FORCE_PATH_STYLE, got error: %v", err)
	}
	if !cfg.S3ForcePathStyle {
		t.Errorf("expected S3ForcePathStyle to default to true on invalid value, got %v", cfg.S3ForcePathStyle)
	}
}

func TestLoadWSTicketSecretDefaultsToJWTSecret(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.WSTicketSecret != cfg.JWTSecret {
		t.Errorf("expected WSTicketSecret to default to JWTSecret %q, got %q", cfg.JWTSecret, cfg.WSTicketSecret)
	}
}

func TestLoadWSTicketSecretOverride(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("WS_TICKET_SECRET", "ws-ticket-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.WSTicketSecret != "ws-ticket-secret" {
		t.Errorf("expected WSTicketSecret override, got %q", cfg.WSTicketSecret)
	}
}

func TestLoadAuthModeDefault(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AuthMode != "simple_jwt" {
		t.Errorf("expected default AuthMode %q, got %q", "simple_jwt", cfg.AuthMode)
	}
}

func TestLoadAuthModeKratos(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("AUTH_MODE", "kratos")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AuthMode != "kratos" {
		t.Errorf("expected AuthMode %q, got %q", "kratos", cfg.AuthMode)
	}
}

func TestLoadAuthModeInvalidReturnsError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("AUTH_MODE", "oidc")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail for an unrecognized AUTH_MODE value")
	}
}

func TestLoadMessageHubDriverDefault(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MessageHubDriver != "inprocess" {
		t.Errorf("expected default MessageHubDriver %q, got %q", "inprocess", cfg.MessageHubDriver)
	}
	if cfg.RedisURL != "" {
		t.Errorf("expected empty RedisURL when MESSAGE_HUB_DRIVER is unset, got %q", cfg.RedisURL)
	}
}

func TestLoadMessageHubDriverRedisRequiresRedisURL(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("MESSAGE_HUB_DRIVER", "redis")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail when MESSAGE_HUB_DRIVER=redis and REDIS_URL is unset")
	}
}

func TestLoadMessageHubDriverRedisWithRedisURL(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("MESSAGE_HUB_DRIVER", "redis")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MessageHubDriver != "redis" {
		t.Errorf("expected MessageHubDriver %q, got %q", "redis", cfg.MessageHubDriver)
	}
	if cfg.RedisURL != "redis://localhost:6379/0" {
		t.Errorf("expected RedisURL to be set, got %q", cfg.RedisURL)
	}
}

func TestLoadMessageHubDriverInvalidReturnsError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("MESSAGE_HUB_DRIVER", "kafka")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail for an unrecognized MESSAGE_HUB_DRIVER value")
	}
}

func TestLoadKratosDefaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.KratosPublicURL != "http://localhost:4433" {
		t.Errorf("expected default KratosPublicURL, got %q", cfg.KratosPublicURL)
	}
	if cfg.KratosAdminURL != "http://localhost:4434" {
		t.Errorf("expected default KratosAdminURL, got %q", cfg.KratosAdminURL)
	}
	if cfg.KratosCookieName != "ory_kratos_session" {
		t.Errorf("expected default KratosCookieName, got %q", cfg.KratosCookieName)
	}
}

func TestLoadLLMGatewayGRPCDefaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LLMGatewayTransport != "rest" {
		t.Errorf("expected default LLMGatewayTransport %q, got %q", "rest", cfg.LLMGatewayTransport)
	}
	if cfg.LLMGatewayGRPCAddr != "llm-gateway:50051" {
		t.Errorf("expected default LLMGatewayGRPCAddr %q, got %q", "llm-gateway:50051", cfg.LLMGatewayGRPCAddr)
	}
	if cfg.LLMGatewayGRPCMaxRetries != 3 {
		t.Errorf("expected default LLMGatewayGRPCMaxRetries 3, got %d", cfg.LLMGatewayGRPCMaxRetries)
	}
	if cfg.LLMGatewayGRPCBaseBackoff != 100*time.Millisecond {
		t.Errorf("expected default LLMGatewayGRPCBaseBackoff 100ms, got %v", cfg.LLMGatewayGRPCBaseBackoff)
	}
}

func TestLoadLLMGatewayGRPCOverrides(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("LLM_GATEWAY_TRANSPORT", "grpc")
	t.Setenv("LLM_GATEWAY_GRPC_ADDR", "localhost:9999")
	t.Setenv("LLM_GATEWAY_GRPC_MAX_RETRIES", "5")
	t.Setenv("LLM_GATEWAY_GRPC_BASE_BACKOFF", "250ms")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LLMGatewayTransport != "grpc" {
		t.Errorf("expected overridden LLMGatewayTransport %q, got %q", "grpc", cfg.LLMGatewayTransport)
	}
	if cfg.LLMGatewayGRPCAddr != "localhost:9999" {
		t.Errorf("expected overridden LLMGatewayGRPCAddr, got %q", cfg.LLMGatewayGRPCAddr)
	}
	if cfg.LLMGatewayGRPCMaxRetries != 5 {
		t.Errorf("expected overridden LLMGatewayGRPCMaxRetries 5, got %d", cfg.LLMGatewayGRPCMaxRetries)
	}
	if cfg.LLMGatewayGRPCBaseBackoff != 250*time.Millisecond {
		t.Errorf("expected overridden LLMGatewayGRPCBaseBackoff 250ms, got %v", cfg.LLMGatewayGRPCBaseBackoff)
	}
}

// TestLoadLLMGatewayTransportInvalidReturnsError is the M2 post-review
// regression test: an unrecognized LLM_GATEWAY_TRANSPORT used to silently
// fall back to "rest" with only a logged warning; it must now fail Load
// hard, matching AUTH_MODE/MESSAGE_HUB_DRIVER's posture (see
// TestLoadAuthModeInvalidReturnsError/TestLoadMessageHubDriverInvalidReturnsError).
func TestLoadLLMGatewayTransportInvalidReturnsError(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("LLM_GATEWAY_TRANSPORT", "carrier-pigeon")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail for an unrecognized LLM_GATEWAY_TRANSPORT value")
	}
}

func TestLoadLLMGatewayGRPCMaxRetriesInvalidFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("LLM_GATEWAY_GRPC_MAX_RETRIES", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on an invalid retry count, got: %v", err)
	}
	if cfg.LLMGatewayGRPCMaxRetries != 3 {
		t.Errorf("expected fallback to default LLMGatewayGRPCMaxRetries 3, got %d", cfg.LLMGatewayGRPCMaxRetries)
	}
}

// TestLoadLLMGatewayGRPCMaxRetriesNegativeFallsBackToDefault is the review
// fix for a negative retry count parsing successfully via strconv.Atoi (it
// is a valid integer, just not a valid retry count) and then misbehaving —
// GRPCClient.callWithRetry only clamps maxRetries < 1 up to 1, so a
// negative value would silently behave like "retry disabled" with no
// warning logged. A negative value must now be treated the same as a parse
// failure: warn and keep the default.
func TestLoadLLMGatewayGRPCMaxRetriesNegativeFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("LLM_GATEWAY_GRPC_MAX_RETRIES", "-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on a negative retry count, got: %v", err)
	}
	if cfg.LLMGatewayGRPCMaxRetries != 3 {
		t.Errorf("expected fallback to default LLMGatewayGRPCMaxRetries 3, got %d", cfg.LLMGatewayGRPCMaxRetries)
	}
}

func TestLoadLLMGatewayGRPCBaseBackoffInvalidFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("LLM_GATEWAY_GRPC_BASE_BACKOFF", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on an invalid backoff duration, got: %v", err)
	}
	if cfg.LLMGatewayGRPCBaseBackoff != 100*time.Millisecond {
		t.Errorf("expected fallback to default LLMGatewayGRPCBaseBackoff 100ms, got %v", cfg.LLMGatewayGRPCBaseBackoff)
	}
}

func TestLoadRateLimitAndWhoamiCacheDefaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.RateLimitLoginPerMinute != 10 {
		t.Errorf("expected default RateLimitLoginPerMinute 10, got %d", cfg.RateLimitLoginPerMinute)
	}
	if cfg.RateLimitAIInvokePerMinute != 20 {
		t.Errorf("expected default RateLimitAIInvokePerMinute 20, got %d", cfg.RateLimitAIInvokePerMinute)
	}
	if cfg.WhoamiCacheTTL != 30*time.Second {
		t.Errorf("expected default WhoamiCacheTTL 30s, got %v", cfg.WhoamiCacheTTL)
	}
}

func TestLoadRateLimitAndWhoamiCacheOverrides(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("RATE_LIMIT_LOGIN_PER_MINUTE", "5")
	t.Setenv("RATE_LIMIT_AI_INVOKE_PER_MINUTE", "50")
	t.Setenv("WHOAMI_CACHE_TTL", "1m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.RateLimitLoginPerMinute != 5 {
		t.Errorf("expected overridden RateLimitLoginPerMinute 5, got %d", cfg.RateLimitLoginPerMinute)
	}
	if cfg.RateLimitAIInvokePerMinute != 50 {
		t.Errorf("expected overridden RateLimitAIInvokePerMinute 50, got %d", cfg.RateLimitAIInvokePerMinute)
	}
	if cfg.WhoamiCacheTTL != time.Minute {
		t.Errorf("expected overridden WhoamiCacheTTL 1m, got %v", cfg.WhoamiCacheTTL)
	}
}

func TestLoadRateLimitInvalidFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("RATE_LIMIT_LOGIN_PER_MINUTE", "not-a-number")
	t.Setenv("RATE_LIMIT_AI_INVOKE_PER_MINUTE", "also-not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on invalid rate-limit values, got: %v", err)
	}
	if cfg.RateLimitLoginPerMinute != 10 {
		t.Errorf("expected fallback to default RateLimitLoginPerMinute 10, got %d", cfg.RateLimitLoginPerMinute)
	}
	if cfg.RateLimitAIInvokePerMinute != 20 {
		t.Errorf("expected fallback to default RateLimitAIInvokePerMinute 20, got %d", cfg.RateLimitAIInvokePerMinute)
	}
}

func TestLoadWhoamiCacheTTLInvalidFallsBackToDefault(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("WHOAMI_CACHE_TTL", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on an invalid WHOAMI_CACHE_TTL, got: %v", err)
	}
	if cfg.WhoamiCacheTTL != 30*time.Second {
		t.Errorf("expected fallback to default WhoamiCacheTTL 30s, got %v", cfg.WhoamiCacheTTL)
	}
}

func TestLoadStripeDefaults(t *testing.T) {
	withRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.StripeSecretKey != "" || cfg.StripeWebhookSecret != "" {
		t.Errorf("expected empty Stripe secret/webhook secret by default, got %q / %q", cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	}
	if len(cfg.StripePlans) != 0 || len(cfg.StripeTokenPackages) != 0 {
		t.Errorf("expected no plans/packages by default, got %v / %v", cfg.StripePlans, cfg.StripeTokenPackages)
	}
	wantSuccess := "http://localhost:3000/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}"
	if cfg.StripeCheckoutSuccessURL != wantSuccess {
		t.Errorf("expected default success url %q, got %q", wantSuccess, cfg.StripeCheckoutSuccessURL)
	}
	wantCancel := "http://localhost:3000/billing/checkout/cancel"
	if cfg.StripeCheckoutCancelURL != wantCancel {
		t.Errorf("expected default cancel url %q, got %q", wantCancel, cfg.StripeCheckoutCancelURL)
	}
}

func TestLoadStripePlansAndPackagesParsed(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_123")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_123")
	t.Setenv("STRIPE_PLANS_JSON", `[{"plan_code":"starter","price_id":"price_1","name":"Starter","description":"d","price_cents":500,"currency":"usd","monthly_token_allocation":100000}]`)
	t.Setenv("STRIPE_TOKEN_PACKAGES_JSON", `[{"package_code":"topup_small","price_id":"price_2","name":"Small","description":"d2","price_cents":300,"currency":"usd","tokens":50000}]`)
	t.Setenv("STRIPE_CHECKOUT_SUCCESS_URL", "https://example.com/success")
	t.Setenv("STRIPE_CHECKOUT_CANCEL_URL", "https://example.com/cancel")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.StripeSecretKey != "sk_test_123" || cfg.StripeWebhookSecret != "whsec_123" {
		t.Errorf("expected Stripe secret/webhook secret to be read from env, got %q / %q", cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	}
	if len(cfg.StripePlans) != 1 || cfg.StripePlans[0].PlanCode != "starter" || cfg.StripePlans[0].MonthlyTokenAllocation != 100000 {
		t.Fatalf("expected 1 parsed plan starter/100000, got %+v", cfg.StripePlans)
	}
	if len(cfg.StripeTokenPackages) != 1 || cfg.StripeTokenPackages[0].PackageCode != "topup_small" || cfg.StripeTokenPackages[0].Tokens != 50000 {
		t.Fatalf("expected 1 parsed package topup_small/50000, got %+v", cfg.StripeTokenPackages)
	}
	if cfg.StripeCheckoutSuccessURL != "https://example.com/success" || cfg.StripeCheckoutCancelURL != "https://example.com/cancel" {
		t.Errorf("expected overridden checkout URLs, got %q / %q", cfg.StripeCheckoutSuccessURL, cfg.StripeCheckoutCancelURL)
	}
}

func TestLoadStripePlansInvalidJSONIgnoredNotFatal(t *testing.T) {
	withRequiredEnv(t)
	t.Setenv("STRIPE_PLANS_JSON", "not-valid-json")
	t.Setenv("STRIPE_TOKEN_PACKAGES_JSON", "also-not-valid-json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on invalid Stripe catalog JSON, got: %v", err)
	}
	if len(cfg.StripePlans) != 0 || len(cfg.StripeTokenPackages) != 0 {
		t.Errorf("expected invalid JSON to be ignored (empty catalogs), got %v / %v", cfg.StripePlans, cfg.StripeTokenPackages)
	}
}
