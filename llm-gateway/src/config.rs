use std::time::Duration;

/// HTTP client tuning shared by all outbound provider adapters.
///
/// Centralizes connect/request timeouts and retry policy so that no adapter needs
/// to hardcode its own `reqwest::Client` timeouts or retry counts.
#[derive(Debug, Clone)]
pub struct HttpClientConfig {
    /// Maximum time to wait while establishing a TCP/TLS connection.
    pub connect_timeout: Duration,
    /// Maximum time to wait for the entire request (connect + send + receive).
    pub request_timeout: Duration,
    /// Maximum number of retry attempts after the initial request on a retryable
    /// (`429`/`5xx`) response.
    pub max_retries: u32,
    /// Base delay used for exponential backoff between retries (see
    /// `RetryPolicy::backoff_delay`).
    pub retry_base_delay: Duration,
}

/// Per-provider configuration that is safe to keep outside `KeyStore`.
///
/// API keys are intentionally excluded from this struct — they are always resolved
/// via `KeyStore` so that credential resolution stays swappable (env vars, Vault, ...)
/// independent of non-secret configuration like the base URL.
#[derive(Debug, Clone)]
pub struct ProviderConfig {
    /// Base URL of the provider's API (e.g. `https://api.openai.com`).
    pub base_url: String,
}

/// Application-wide configuration loaded from environment variables.
///
/// This is the single source of truth for HTTP client tuning and per-provider base
/// URLs. Adapters must not read `std::env` directly for any of these values — they
/// receive an owned `HttpClientConfig`/`ProviderConfig` instead, so future provider
/// adapters (Anthropic, Gemini, ...) can reuse the same plumbing.
///
/// Provider API keys are NOT included here. Each adapter retrieves those via `KeyStore`.
#[derive(Debug, Clone)]
pub struct Config {
    /// Port the HTTP (REST) server listens on.
    pub port: u16,
    /// Port the gRPC server listens on.
    pub grpc_port: u16,
    /// Shared HTTP client tuning (timeouts, retry policy) for all outbound adapters.
    pub http: HttpClientConfig,
    /// OpenAI-specific configuration (base URL).
    pub openai: ProviderConfig,
    /// Anthropic-specific configuration (base URL).
    pub anthropic: ProviderConfig,
    /// Gemini-specific configuration (base URL).
    pub gemini: ProviderConfig,
}

impl Config {
    /// Loads configuration from environment variables.
    ///
    /// # Environment Variables
    /// - `LLM_GATEWAY_PORT` — REST listen port (default: `8081`)
    /// - `LLM_GATEWAY_GRPC_PORT` — gRPC listen port (default: `50051`)
    /// - `LLM_GATEWAY_CONNECT_TIMEOUT_SECS` — Connect timeout in seconds (default: `10`)
    /// - `LLM_GATEWAY_REQUEST_TIMEOUT_SECS` — Total request timeout in seconds (default: `600`,
    ///   since LLM completions routinely take longer than a typical HTTP request timeout)
    /// - `LLM_GATEWAY_MAX_RETRIES` — Max retry attempts on `429`/`5xx` responses (default: `3`)
    /// - `LLM_GATEWAY_RETRY_BASE_DELAY_MS` — Base backoff delay in milliseconds (default: `500`)
    /// - `OPENAI_BASE_URL` — OpenAI API base URL (default: `https://api.openai.com`)
    /// - `ANTHROPIC_BASE_URL` — Anthropic API base URL (default: `https://api.anthropic.com`)
    /// - `GEMINI_BASE_URL` — Gemini API base URL (default: `https://generativelanguage.googleapis.com`)
    ///
    /// # Returns
    /// A `Config` populated from the environment, falling back to defaults for any
    /// variable that is unset or fails to parse.
    pub fn from_env() -> Self {
        let port = env_parsed("LLM_GATEWAY_PORT", 8081);
        let grpc_port = env_parsed("LLM_GATEWAY_GRPC_PORT", 50051);

        let connect_timeout =
            Duration::from_secs(env_parsed("LLM_GATEWAY_CONNECT_TIMEOUT_SECS", 10));
        let request_timeout =
            Duration::from_secs(env_parsed("LLM_GATEWAY_REQUEST_TIMEOUT_SECS", 600));
        let max_retries = env_parsed("LLM_GATEWAY_MAX_RETRIES", 3);
        let retry_base_delay =
            Duration::from_millis(env_parsed("LLM_GATEWAY_RETRY_BASE_DELAY_MS", 500));

        let base_url = std::env::var("OPENAI_BASE_URL")
            .unwrap_or_else(|_| "https://api.openai.com".to_string());
        let anthropic_base_url = std::env::var("ANTHROPIC_BASE_URL")
            .unwrap_or_else(|_| "https://api.anthropic.com".to_string());
        let gemini_base_url = std::env::var("GEMINI_BASE_URL")
            .unwrap_or_else(|_| "https://generativelanguage.googleapis.com".to_string());

        Self {
            port,
            grpc_port,
            http: HttpClientConfig {
                connect_timeout,
                request_timeout,
                max_retries,
                retry_base_delay,
            },
            openai: ProviderConfig { base_url },
            anthropic: ProviderConfig {
                base_url: anthropic_base_url,
            },
            gemini: ProviderConfig {
                base_url: gemini_base_url,
            },
        }
    }
}

/// Reads an environment variable and parses it, falling back to `default` if the
/// variable is unset or fails to parse.
///
/// # Arguments
/// * `key` — Environment variable name.
/// * `default` — Value to use when the variable is unset or unparseable.
///
/// # Returns
/// The parsed value, or `default`.
fn env_parsed<T: std::str::FromStr>(key: &str, default: T) -> T {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Environment variables are process-global, so serialize tests that mutate them
    // to avoid cross-test interference (mirrors the guard pattern used elsewhere in
    // this crate for env-var-based tests).
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    /// Environment variable names touched by `Config::from_env` tests.
    const CONFIG_ENV_VARS: &[&str] = &[
        "LLM_GATEWAY_PORT",
        "LLM_GATEWAY_GRPC_PORT",
        "LLM_GATEWAY_CONNECT_TIMEOUT_SECS",
        "LLM_GATEWAY_REQUEST_TIMEOUT_SECS",
        "LLM_GATEWAY_MAX_RETRIES",
        "LLM_GATEWAY_RETRY_BASE_DELAY_MS",
        "OPENAI_BASE_URL",
        "ANTHROPIC_BASE_URL",
        "GEMINI_BASE_URL",
    ];

    /// RAII guard that snapshots `CONFIG_ENV_VARS`, clears them for the duration of
    /// a test, and restores their original values (or unset-ness) on drop.
    ///
    /// This makes env-var state exception-safe: a panic or early return inside the
    /// test body still triggers `Drop::drop`, so a failing test can't leak env
    /// mutations into whichever test runs next (tests share a process-global
    /// environment, and are serialized via `ENV_LOCK` to make that safe).
    struct EnvVarGuard {
        snapshot: Vec<(&'static str, Option<String>)>,
    }

    impl EnvVarGuard {
        /// Snapshots the current value of every `CONFIG_ENV_VARS` entry, then
        /// removes them all from the environment so tests start from a clean slate.
        fn new() -> Self {
            let snapshot: Vec<(&'static str, Option<String>)> = CONFIG_ENV_VARS
                .iter()
                .map(|&key| (key, std::env::var(key).ok()))
                .collect();

            for &key in CONFIG_ENV_VARS {
                unsafe {
                    std::env::remove_var(key);
                }
            }

            Self { snapshot }
        }
    }

    impl Drop for EnvVarGuard {
        /// Restores every snapshotted variable to its original value, or leaves it
        /// unset if it was unset when the guard was created.
        fn drop(&mut self) {
            for (key, value) in &self.snapshot {
                unsafe {
                    match value {
                        Some(v) => std::env::set_var(key, v),
                        None => std::env::remove_var(key),
                    }
                }
            }
        }
    }

    #[test]
    fn test_from_env_defaults() {
        let _lock = ENV_LOCK.lock().unwrap();
        let _guard = EnvVarGuard::new();

        let config = Config::from_env();

        assert_eq!(config.port, 8081);
        assert_eq!(config.grpc_port, 50051);
        assert_eq!(config.http.connect_timeout, Duration::from_secs(10));
        assert_eq!(config.http.request_timeout, Duration::from_secs(600));
        assert_eq!(config.http.max_retries, 3);
        assert_eq!(config.http.retry_base_delay, Duration::from_millis(500));
        assert_eq!(config.openai.base_url, "https://api.openai.com");
        assert_eq!(config.anthropic.base_url, "https://api.anthropic.com");
        assert_eq!(
            config.gemini.base_url,
            "https://generativelanguage.googleapis.com"
        );
    }

    #[test]
    fn test_from_env_overrides() {
        let _lock = ENV_LOCK.lock().unwrap();
        let _guard = EnvVarGuard::new();

        unsafe {
            std::env::set_var("LLM_GATEWAY_PORT", "9000");
            std::env::set_var("LLM_GATEWAY_GRPC_PORT", "50052");
            std::env::set_var("LLM_GATEWAY_CONNECT_TIMEOUT_SECS", "5");
            std::env::set_var("LLM_GATEWAY_REQUEST_TIMEOUT_SECS", "60");
            std::env::set_var("LLM_GATEWAY_MAX_RETRIES", "5");
            std::env::set_var("LLM_GATEWAY_RETRY_BASE_DELAY_MS", "100");
            std::env::set_var("OPENAI_BASE_URL", "http://localhost:9091");
            std::env::set_var("ANTHROPIC_BASE_URL", "http://localhost:9093");
            std::env::set_var("GEMINI_BASE_URL", "http://localhost:9092");
        }

        let config = Config::from_env();

        assert_eq!(config.port, 9000);
        assert_eq!(config.grpc_port, 50052);
        assert_eq!(config.http.connect_timeout, Duration::from_secs(5));
        assert_eq!(config.http.request_timeout, Duration::from_secs(60));
        assert_eq!(config.http.max_retries, 5);
        assert_eq!(config.http.retry_base_delay, Duration::from_millis(100));
        assert_eq!(config.openai.base_url, "http://localhost:9091");
        assert_eq!(config.anthropic.base_url, "http://localhost:9093");
        assert_eq!(config.gemini.base_url, "http://localhost:9092");
    }
}
