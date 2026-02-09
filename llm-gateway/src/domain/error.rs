use std::fmt;

/// Domain-layer error type.
///
/// Represents all business logic errors. Converted to HTTP status codes etc. in the adapter layer.
#[derive(Debug)]
pub enum DomainError {
    /// Error response from an LLM provider.
    ProviderError(String),
    /// Invalid request parameters.
    InvalidRequest(String),
    /// The requested model was not found.
    ModelNotFound(String),
    /// API key not found for a provider.
    KeyNotFound(String),
    /// Request to a provider timed out.
    Timeout,
}

impl fmt::Display for DomainError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::ProviderError(msg) => write!(f, "provider error: {msg}"),
            Self::InvalidRequest(msg) => write!(f, "invalid request: {msg}"),
            Self::ModelNotFound(model) => write!(f, "model not found: {model}"),
            Self::KeyNotFound(provider) => {
                write!(f, "API key not found for provider: {provider}")
            }
            Self::Timeout => write!(f, "request timed out"),
        }
    }
}

impl std::error::Error for DomainError {}
