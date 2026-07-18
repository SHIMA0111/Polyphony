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

/// Anthropic Messages API version sent with every request via the required
/// `anthropic-version` header.
///
/// Anthropic (unlike OpenAI) versions its API through this header rather than through
/// the URL path, so this constant must be kept in sync with the version this adapter's
/// request/response mapping was written against.
pub(super) const ANTHROPIC_VERSION: &str = "2023-06-01";

/// Statically cached list of models offered by the Anthropic adapter.
///
/// Populated once on first access via `OnceLock` and cloned on each `models()` call,
/// since the list never changes at runtime.
static MODELS: OnceLock<Vec<ModelInfo>> = OnceLock::new();

fn models_list() -> &'static Vec<ModelInfo> {
    // Metadata beyond id/name/provider/owned_by (context window, pricing, image
    // support) is intentionally left `None` here — Step 34 populates it from an
    // authoritative source.
    MODELS.get_or_init(|| {
        vec![
            ModelInfo {
                id: "claude-opus-4-6".to_string(),
                name: "Claude Opus 4.6".to_string(),
                provider: "anthropic".to_string(),
                owned_by: "anthropic".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
            ModelInfo {
                id: "claude-sonnet-4-6".to_string(),
                name: "Claude Sonnet 4.6".to_string(),
                provider: "anthropic".to_string(),
                owned_by: "anthropic".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
            ModelInfo {
                id: "claude-haiku-4-6".to_string(),
                name: "Claude Haiku 4.6".to_string(),
                provider: "anthropic".to_string(),
                owned_by: "anthropic".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            },
        ]
    })
}

/// Anthropic Messages API adapter.
///
/// API keys are retrieved via `KeyStore` **lazily, per request** rather than at
/// construction time: this lets the gateway process start and serve `GET /health`
/// even when the key is not yet resolvable, so `GET /ready` (see
/// `CompletionService::readiness`) is the only signal that genuinely distinguishes
/// "process is up" from "dependencies are usable". All other configuration (base URL,
/// HTTP client timeouts, retry policy) is injected explicitly via `Config` at
/// construction time — this adapter never reads `std::env` directly.
pub struct AnthropicProvider {
    /// HTTP client used to send requests to the Anthropic Messages API.
    client: Client,
    /// Base URL for the Anthropic Messages API (e.g. `https://api.anthropic.com`).
    base_url: String,
    /// Key store used to resolve the API key lazily, per request (see the struct-level
    /// docs above).
    key_store: Arc<dyn KeyStore>,
    /// Retry policy applied to transient failures (429/5xx, including Anthropic's `529
    /// overloaded_error`) via `send_with_retry`.
    retry_policy: RetryPolicy,
}

impl AnthropicProvider {
    const PROVIDER_NAME: &'static str = "anthropic";

    /// Creates a new `AnthropicProvider`.
    ///
    /// # Arguments
    /// * `key_store` — Key store used to resolve the API key on each request (not
    ///   eagerly here), so gateway startup never depends on the key being present.
    /// * `http` — Shared HTTP client tuning (connect/request timeouts, retry policy).
    /// * `provider` — Anthropic-specific configuration (base URL).
    ///
    /// # Returns
    /// A ready-to-use `AnthropicProvider` wrapping a configured `reqwest::Client` and
    /// the supplied `key_store`/`base_url`/retry policy.
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

impl LLMProvider for AnthropicProvider {
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
