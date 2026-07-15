use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest};

use super::OpenAIProvider;

/// Executes a streaming chat completion request against the OpenAI Chat Completions API.
///
/// # Arguments
/// * `_provider` — The `OpenAIProvider` that would perform the streaming HTTP call.
/// * `_req` — Completion request that would be streamed.
///
/// # Errors
/// Always returns `DomainError::ProviderError`: SSE parsing into `CompletionChunk`s is
/// not yet implemented for this adapter.
// TODO: implement SSE parsing of OpenAI's streaming Chat Completions response into
// `CompletionChunk`s once the provider's HTTP client supports it (see phases.md streaming phase).
// Not yet called from any inbound adapter — allowed dead code until a later step wires
// a real streaming HTTP endpoint on top of `LLMProvider::stream`.
#[allow(dead_code)]
pub(super) fn stream<'a>(
    _provider: &'a OpenAIProvider,
    _req: &CompletionRequest,
) -> BoxFuture<'a, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>> {
    Box::pin(async move {
        Err(DomainError::provider_error(
            "streaming is not yet implemented for the OpenAI adapter",
        ))
    })
}
