use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo};

/// LLM provider port.
///
/// Trait implemented by each LLM provider adapter (OpenAI, Anthropic, Gemini, etc.).
/// Adding a new provider only requires implementing this trait.
pub trait LLMProvider: Send + Sync {
    /// Executes a chat completion request.
    ///
    /// # Arguments
    /// * `req` — Completion request
    ///
    /// # Errors
    /// Returns `DomainError` on provider errors, timeouts, etc.
    fn complete(
        &self,
        req: &CompletionRequest,
    ) -> BoxFuture<'_, Result<CompletionResponse, DomainError>>;

    /// Returns the list of models provided by this provider.
    ///
    /// # Returns
    /// A future resolving to the models available from this provider.
    fn models(&self) -> BoxFuture<'_, Vec<ModelInfo>>;

    /// Executes a streaming chat completion request.
    ///
    /// # Arguments
    /// * `req` — Completion request
    ///
    /// # Returns
    /// A future resolving to a stream of `CompletionChunk`s as the provider produces them.
    ///
    /// # Errors
    /// Returns `DomainError` if the stream cannot be established (e.g. provider errors,
    /// timeouts) or is not supported by this provider.
    // Not yet called from any inbound adapter — a later step wires a real streaming
    // HTTP endpoint on top of this port. Kept here so provider adapters can implement it now.
    #[allow(dead_code)]
    fn stream(
        &self,
        req: &CompletionRequest,
    ) -> BoxFuture<'_, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>>;

    /// Returns the provider name (e.g. "openai").
    fn provider_name(&self) -> &str;
}
