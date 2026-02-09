use crate::domain::error::DomainError;
use crate::ports::outbound::key_store::KeyStore;

/// Key store that retrieves API keys from environment variables.
///
/// Reads keys from `{PROVIDER}_API_KEY` (uppercased) environment variables.
/// Example: provider="openai" → `OPENAI_API_KEY`
pub struct EnvKeyStore;

impl KeyStore for EnvKeyStore {
    fn get_key(&self, provider: &str) -> Result<String, DomainError> {
        let var_name = format!("{}_API_KEY", provider.to_uppercase());
        std::env::var(&var_name).map_err(|_| DomainError::KeyNotFound(provider.to_string()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_get_key_from_env() {
        unsafe {
            std::env::set_var("TESTPROV_API_KEY", "test-key-123");
        }
        let store = EnvKeyStore;
        let key = store.get_key("testprov").unwrap();
        assert_eq!(key, "test-key-123");
        unsafe {
            std::env::remove_var("TESTPROV_API_KEY");
        }
    }

    #[test]
    fn test_key_not_found() {
        let store = EnvKeyStore;
        let result = store.get_key("nonexistent_provider_xyz");
        assert!(matches!(result, Err(DomainError::KeyNotFound(_))));
    }
}
