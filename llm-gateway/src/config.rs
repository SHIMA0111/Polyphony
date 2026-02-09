/// Application-wide configuration loaded from environment variables.
///
/// Provider-specific settings (API keys, base URLs, etc.) are NOT included here.
/// Each adapter retrieves those via `KeyStore` or its own environment variables.
#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
}

impl Config {
    /// Loads configuration from environment variables.
    ///
    /// # Environment Variables
    /// - `LLM_GATEWAY_PORT` — Listen port (default: 8081)
    pub fn from_env() -> Self {
        let port = std::env::var("LLM_GATEWAY_PORT")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(8081);

        Self { port }
    }
}
