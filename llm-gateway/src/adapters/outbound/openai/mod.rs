mod request;
mod stream;

use std::sync::{Arc, OnceLock};

use futures::future::BoxFuture;
use futures::stream::BoxStream;
use reqwest::Client;

use crate::adapters::outbound::http_retry::RetryPolicy;
use crate::config::{HttpClientConfig, ProviderConfig};
use crate::domain::error::DomainError;
use crate::domain::model::{
    CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo, ModelPricing,
};
use crate::ports::outbound::key_store::KeyStore;
use crate::ports::outbound::provider::LLMProvider;

/// Statically cached list of models offered by the OpenAI adapter.
///
/// Populated once on first access via `OnceLock` and cloned on each `models()` call,
/// since the list never changes at runtime.
static MODELS: OnceLock<Vec<ModelInfo>> = OnceLock::new();

fn models_list() -> &'static Vec<ModelInfo> {
    // Illustrative placeholder metadata — not verified real-world pricing or context
    // windows; replace with an authoritative source when billing (Phase 16-17) lands.
    MODELS.get_or_init(|| {
        vec![
            ModelInfo {
                id: "gpt-5.2".to_string(),
                name: "GPT-5.2".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
                context_window: Some(400_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 2.5,
                    output_price_per_million_tokens: 10.0,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            },
            ModelInfo {
                id: "gpt-5".to_string(),
                name: "GPT-5".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
                context_window: Some(272_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 1.25,
                    output_price_per_million_tokens: 10.0,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            },
            ModelInfo {
                id: "gpt-5-mini".to_string(),
                name: "GPT-5 Mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
                context_window: Some(272_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 0.25,
                    output_price_per_million_tokens: 2.0,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            },
            ModelInfo {
                id: "o4-mini".to_string(),
                name: "o4-mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
                context_window: Some(200_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 1.1,
                    output_price_per_million_tokens: 4.4,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            },
            ModelInfo {
                id: "o3".to_string(),
                name: "o3".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
                context_window: Some(200_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 2.0,
                    output_price_per_million_tokens: 8.0,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            },
        ]
    })
}

/// OpenAI Chat Completions API adapter.
///
/// API keys are retrieved via `KeyStore` **lazily, per request** rather than at
/// construction time: this lets the gateway process start and serve `GET /health`
/// even when the key is not yet resolvable, so `GET /ready` (see
/// `CompletionService::readiness`) is the only signal that genuinely distinguishes
/// "process is up" from "dependencies are usable". All other configuration (base URL,
/// HTTP client timeouts, retry policy) is injected explicitly via `Config` at
/// construction time — this adapter never reads `std::env` directly.
pub struct OpenAIProvider {
    client: Client,
    base_url: String,
    key_store: Arc<dyn KeyStore>,
    retry_policy: RetryPolicy,
}

impl OpenAIProvider {
    const PROVIDER_NAME: &'static str = "openai";

    /// Creates a new `OpenAIProvider`.
    ///
    /// # Arguments
    /// * `key_store` — Key store used to resolve the API key on each request (not
    ///   eagerly here), so gateway startup never depends on the key being present.
    /// * `http` — Shared HTTP client tuning (connect/request timeouts, retry policy).
    /// * `provider` — OpenAI-specific configuration (base URL).
    ///
    /// # Errors
    /// Returns `DomainError::ProviderError` if the underlying `reqwest::Client` fails
    /// to build. Never fails due to a missing API key — that is only reported when a
    /// request is actually made (via `complete`/`stream`) or via
    /// `CompletionUseCase::readiness`.
    pub fn new(
        key_store: Arc<dyn KeyStore>,
        http: HttpClientConfig,
        provider: ProviderConfig,
    ) -> Result<Self, DomainError> {
        let client = Client::builder()
            .connect_timeout(http.connect_timeout)
            .timeout(http.request_timeout)
            .build()
            .map_err(|e| {
                DomainError::provider_error_with_source("failed to create reqwest client", e)
            })?;

        Ok(Self {
            client,
            base_url: provider.base_url,
            key_store,
            retry_policy: RetryPolicy::new(&http),
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
