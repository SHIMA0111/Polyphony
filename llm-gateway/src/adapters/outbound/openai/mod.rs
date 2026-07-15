mod request;
mod stream;

use std::sync::{Arc, OnceLock};

use futures::future::BoxFuture;
use futures::stream::BoxStream;
use reqwest::Client;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo};
use crate::ports::outbound::key_store::KeyStore;
use crate::ports::outbound::provider::LLMProvider;

/// Statically cached list of models offered by the OpenAI adapter.
///
/// Populated once on first access via `OnceLock` and cloned on each `models()` call,
/// since the list never changes at runtime.
static MODELS: OnceLock<Vec<ModelInfo>> = OnceLock::new();

fn models_list() -> &'static Vec<ModelInfo> {
    MODELS.get_or_init(|| {
        vec![
            ModelInfo {
                id: "gpt-5.2".to_string(),
                name: "GPT-5.2".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "gpt-5".to_string(),
                name: "GPT-5".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "gpt-5-mini".to_string(),
                name: "GPT-5 Mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "o4-mini".to_string(),
                name: "o4-mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "o3".to_string(),
                name: "o3".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
        ]
    })
}

/// OpenAI Chat Completions API adapter.
///
/// API keys are retrieved via `KeyStore` and the base URL is read from the
/// `OPENAI_BASE_URL` environment variable. Provider-specific configuration is
/// encapsulated within this adapter and not included in the shared Config.
pub struct OpenAIProvider {
    client: Client,
    base_url: String,
    api_key: String,
}

impl OpenAIProvider {
    const PROVIDER_NAME: &'static str = "openai";

    /// Creates a new `OpenAIProvider`.
    ///
    /// # Arguments
    /// * `key_store` — Key store used to retrieve the API key
    ///
    /// # Environment Variables
    /// * `OPENAI_BASE_URL` — OpenAI API base URL (default: https://api.openai.com)
    pub fn new(key_store: Arc<dyn KeyStore>) -> Result<Self, DomainError> {
        let base_url = std::env::var("OPENAI_BASE_URL")
            .unwrap_or_else(|_| "https://api.openai.com".to_string());
        let api_key = key_store.get_key(Self::PROVIDER_NAME)?;

        let client = Client::builder()
            // To avoid connection issues like misconfiguration or network failures, we set a timeout of 10 seconds
            .connect_timeout(std::time::Duration::from_secs(10))
            .build()
            .map_err(|e| {
                DomainError::provider_error_with_source("failed to create reqwest client", e)
            })?;

        Ok(Self {
            client,
            base_url,
            api_key,
        })
    }
}

impl LLMProvider for OpenAIProvider {
    fn complete(
        &self,
        req: &CompletionRequest,
    ) -> BoxFuture<'_, Result<CompletionResponse, DomainError>> {
        request::complete(self, req)
    }

    fn models(&self) -> BoxFuture<'_, Vec<ModelInfo>> {
        Box::pin(async move { models_list().clone() })
    }

    fn stream(
        &self,
        req: &CompletionRequest,
    ) -> BoxFuture<'_, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>>
    {
        stream::stream(self, req)
    }

    fn provider_name(&self) -> &str {
        Self::PROVIDER_NAME
    }
}
