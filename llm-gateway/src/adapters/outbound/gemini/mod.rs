mod request;
mod stream;

use std::sync::{Arc, OnceLock};

use futures::future::BoxFuture;
use futures::stream::BoxStream;
use reqwest::Client;

use crate::adapters::outbound::http_retry::RetryPolicy;
use crate::config::{HttpClientConfig, ProviderConfig};
use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo};
use crate::ports::outbound::key_store::KeyStore;
use crate::ports::outbound::provider::LLMProvider;

/// Statically cached list of models offered by the Gemini adapter.
///
/// Populated once on first access via `OnceLock` and cloned on each `models()` call,
/// since the list never changes at runtime. All IDs use a `gemini-` prefix by
/// convention so they cannot collide with OpenAI's `gpt-`/`o`-prefixed IDs or
/// Anthropic's `claude-`-prefixed IDs. `context_window`/`pricing`/`supports_image_input`
/// are left `None` here — that metadata is populated by a later step (Phase 34).
static MODELS: OnceLock<Vec<ModelInfo>> = OnceLock::new();

fn models_list() -> &'static Vec<ModelInfo> {
    MODELS.get_or_init(|| {
        vec![
            ModelInfo {
                id: "gemini-3-pro".to_string(),
                name: "Gemini 3 Pro".to_string(),
                provider: "gemini".to_string(),
                owned_by: "google".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
            ModelInfo {
                id: "gemini-3-flash".to_string(),
                name: "Gemini 3 Flash".to_string(),
                provider: "gemini".to_string(),
                owned_by: "google".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
            ModelInfo {
                id: "gemini-2.5-flash".to_string(),
                name: "Gemini 2.5 Flash".to_string(),
                provider: "gemini".to_string(),
                owned_by: "google".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
        ]
    })
}

/// Google Generative Language API (`generateContent`) adapter.
///
/// API keys are retrieved via `KeyStore` **lazily, per request** rather than at
/// construction time: this lets the gateway process start and serve `GET /health`
/// even when the key is not yet resolvable, so `GET /ready` (see
/// `CompletionService::readiness`) is the only signal that genuinely distinguishes
/// "process is up" from "dependencies are usable". All other configuration (base URL,
/// HTTP client timeouts, retry policy) is injected explicitly via `Config` at
/// construction time — this adapter never reads `std::env` directly.
pub struct GeminiProvider {
    client: Client,
    base_url: String,
    key_store: Arc<dyn KeyStore>,
    retry_policy: RetryPolicy,
}

impl GeminiProvider {
    const PROVIDER_NAME: &'static str = "gemini";

    /// Creates a new `GeminiProvider`.
    ///
    /// # Arguments
    /// * `key_store` — Key store used to resolve the API key on each request (not
    ///   eagerly here), so gateway startup never depends on the key being present.
    /// * `http` — Shared HTTP client tuning (connect/request timeouts, retry policy).
    /// * `provider` — Gemini-specific configuration (base URL).
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

impl LLMProvider for GeminiProvider {
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
