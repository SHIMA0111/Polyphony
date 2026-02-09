use std::future::Future;
use std::pin::Pin;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionRequest, CompletionResponse, ModelInfo};

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
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>>;

    /// Returns the list of models provided by this provider.
    fn models(&self) -> Vec<ModelInfo>;

    /// Returns the provider name (e.g. "openai").
    fn provider_name(&self) -> &str;
}
