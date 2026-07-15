/// Domain-layer error type.
///
/// Represents all business logic errors. Converted to HTTP status codes etc. in the adapter layer.
#[derive(Debug, thiserror::Error)]
pub enum DomainError {
    /// Error response from an LLM provider.
    #[error("provider error: {message}")]
    ProviderError {
        /// Human-readable description of the failure.
        message: String,
        /// Underlying cause, if any. Boxed as a trait object so the domain layer
        /// never depends on a concrete adapter crate (e.g. `reqwest`, `serde_json`).
        #[source]
        source: Option<Box<dyn std::error::Error + Send + Sync>>,
    },
    /// Invalid request parameters.
    #[error("invalid request: {0}")]
    InvalidRequest(String),
    /// The requested model was not found.
    #[error("model not found: {0}")]
    ModelNotFound(String),
    /// API key not found for a provider.
    #[error("API key not found for provider: {0}")]
    KeyNotFound(String),
    /// Request to a provider timed out.
    #[error("request timed out")]
    Timeout,
    /// The provider rejected the request due to rate limiting (HTTP 429).
    #[error("rate limited{}", retry_after_secs.map(|s| format!(", retry after {s}s")).unwrap_or_default())]
    RateLimited {
        /// Seconds to wait before retrying, if the provider supplied one (e.g. via `Retry-After`).
        retry_after_secs: Option<u64>,
    },
}

impl DomainError {
    /// Builds a `ProviderError` without an underlying cause.
    ///
    /// # Arguments
    /// * `message` — Human-readable description of the failure.
    ///
    /// # Returns
    /// A `DomainError::ProviderError` with `source` set to `None`.
    pub fn provider_error(message: impl Into<String>) -> Self {
        Self::ProviderError {
            message: message.into(),
            source: None,
        }
    }

    /// Builds a `ProviderError` that preserves an underlying cause in its source chain.
    ///
    /// # Arguments
    /// * `message` — Human-readable description of the failure.
    /// * `source` — The underlying error (e.g. a `reqwest::Error` or `serde_json::Error`)
    ///   boxed as a trait object so the domain layer does not depend on the adapter crate.
    ///
    /// # Returns
    /// A `DomainError::ProviderError` with `source` set to `Some(Box::new(source))`.
    pub fn provider_error_with_source(
        message: impl Into<String>,
        source: impl std::error::Error + Send + Sync + 'static,
    ) -> Self {
        Self::ProviderError {
            message: message.into(),
            source: Some(Box::new(source)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug, thiserror::Error)]
    #[error("underlying failure")]
    struct FakeSource;

    #[test]
    fn test_provider_error_has_no_source() {
        let err = DomainError::provider_error("something went wrong");
        assert!(std::error::Error::source(&err).is_none());
    }

    #[test]
    fn test_provider_error_with_source_preserves_source() {
        let err = DomainError::provider_error_with_source("upstream failed", FakeSource);
        assert!(std::error::Error::source(&err).is_some());
    }
}
