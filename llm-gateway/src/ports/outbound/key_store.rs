use crate::domain::error::DomainError;

/// API key retrieval port.
///
/// Retrieves API keys by provider name. Can be backed by environment variables, Vault, etc.
pub trait KeyStore: Send + Sync {
    /// Retrieves the API key for the specified provider.
    ///
    /// # Arguments
    /// * `provider` — Provider name (e.g. "openai")
    ///
    /// # Errors
    /// Returns `DomainError::KeyNotFound` if the key is not found.
    fn get_key(&self, provider: &str) -> Result<String, DomainError>;
}
