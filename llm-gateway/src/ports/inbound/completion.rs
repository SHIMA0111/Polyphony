use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo};

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
}
