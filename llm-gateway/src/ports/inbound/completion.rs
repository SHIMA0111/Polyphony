use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{
    CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo, TokenEstimateRequest,
    TokenEstimateResponse,
};

/// Completion use case port.
///
/// Interface for inbound adapters (REST, gRPC, etc.) to invoke the domain service.
pub trait CompletionUseCase: Send + Sync {
    /// Executes a chat completion.
    ///
    /// # Arguments
    /// * `req` — Completion request
    ///
    /// # Returns
    /// A future resolving to the completion response.
    ///
    /// # Errors
    /// Returns `DomainError` on model not found, provider errors, etc.
    fn complete(
        &self,
        req: CompletionRequest,
    ) -> BoxFuture<'_, Result<CompletionResponse, DomainError>>;

    /// Returns a list of all available models across all providers.
    ///
    /// # Returns
    /// A future resolving to the aggregated model list.
    fn list_models(&self) -> BoxFuture<'_, Vec<ModelInfo>>;

    /// Executes a streaming chat completion.
    ///
    /// # Arguments
    /// * `req` — Completion request
    ///
    /// # Returns
    /// A future resolving to a stream of `CompletionChunk`s as the provider produces them.
    ///
    /// # Errors
    /// Returns `DomainError` if no provider matches the requested model, or the stream
    /// cannot be established.
    // Not yet called from any inbound adapter — a later step wires a real streaming
    // HTTP endpoint on top of this port.
    #[allow(dead_code)]
    fn stream(
        &self,
        req: CompletionRequest,
    ) -> BoxFuture<'_, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>>;

    /// Checks whether the gateway's dependencies are actually usable.
    ///
    /// Unlike a liveness check, this confirms that every registered provider's API
    /// key can be resolved via `KeyStore`. It makes no network calls, so it stays
    /// fast enough to back a `GET /ready` endpoint.
    ///
    /// # Returns
    /// `Ok(())` when every registered provider's API key resolves successfully via
    /// `KeyStore`.
    ///
    /// # Errors
    /// Returns `DomainError::KeyNotFound` for the first registered provider whose API
    /// key cannot be resolved.
    fn readiness(&self) -> Result<(), DomainError>;

    /// Estimates the token count for a list of chat messages.
    ///
    /// This reuses the same `AppState = Arc<dyn CompletionUseCase>` as `/completions`
    /// and `/models` rather than introducing a second state type: the REST router's
    /// `/tokens/estimate` handler is registered against the same shared state.
    ///
    /// Unlike `complete`/`list_models`/`stream`, this method is **synchronous** — the
    /// underlying character-based heuristic (`domain::token_estimator`) performs no
    /// I/O and never fails, so no `BoxFuture`/`Result` wrapping is needed.
    ///
    /// # Arguments
    /// * `req` — The messages (and target model, for future model-specific tuning) to
    ///   estimate.
    ///
    /// # Returns
    /// A `TokenEstimateResponse` carrying the approximate token count.
    fn estimate_tokens(&self, req: TokenEstimateRequest) -> TokenEstimateResponse;
}
