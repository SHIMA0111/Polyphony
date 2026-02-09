use std::future::Future;
use std::pin::Pin;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionRequest, CompletionResponse, ModelInfo};

/// Completion use case port.
///
/// Interface for inbound adapters (REST, gRPC, etc.) to invoke the domain service.
pub trait CompletionUseCase: Send + Sync {
    /// Executes a chat completion.
    ///
    /// # Arguments
    /// * `req` — Completion request
    ///
    /// # Errors
    /// Returns `DomainError` on model not found, provider errors, etc.
    fn complete(
        &self,
        req: CompletionRequest,
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>>;

    /// Returns a list of all available models across all providers.
    fn list_models(&self) -> Vec<ModelInfo>;
}
