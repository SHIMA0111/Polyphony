use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest};

use super::GeminiProvider;

/// Executes a streaming chat completion request against Gemini's `generateContent` API.
///
/// # Arguments
/// * `_provider` — The `GeminiProvider` that would perform the streaming HTTP call.
/// * `_req` — Completion request that would be streamed.
///
/// # Errors
/// Always returns `DomainError::ProviderError`: parsing Gemini's `streamGenerateContent`
/// SSE responses into `CompletionChunk`s is not yet implemented for this adapter.
// TODO: implement SSE parsing of Gemini's `streamGenerateContent` response into
// `CompletionChunk`s once the provider's HTTP client supports it (see phases.md streaming
// phase / Step 43). Not yet called from any inbound adapter — allowed dead code until a
// later step wires a real streaming HTTP endpoint on top of `LLMProvider::stream`.
#[allow(dead_code)]
pub(super) fn stream<'a>(
    _provider: &'a GeminiProvider,
    _req: &CompletionRequest,
) -> BoxFuture<'a, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>> {
    Box::pin(async move {
        Err(DomainError::provider_error(
            "streaming is not yet implemented for the Gemini adapter",
        ))
    })
}
