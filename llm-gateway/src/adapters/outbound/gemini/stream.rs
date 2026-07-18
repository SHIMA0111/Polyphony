use async_stream::try_stream;
use eventsource_stream::Eventsource;
use futures::future::BoxFuture;
use futures::stream::BoxStream;
use serde::Deserialize;

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, Usage};

use super::GeminiProvider;
use super::request::to_gemini_request;

/// Shape of a single Gemini `streamGenerateContent` SSE event's `data` payload.
///
/// Mirrors `request.rs`'s non-streaming `GeminiResponse`, including `response_id`
/// (deserialized from the wire's camelCase `responseId`, same as the non-streaming
/// struct): when a streaming event carries it, this adapter prefers it over the
/// synthetic-`id` fallback documented on [`stream`].
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiStreamEvent {
    #[serde(default)]
    candidates: Vec<GeminiStreamCandidate>,
    usage_metadata: Option<GeminiStreamUsageMetadata>,
    response_id: Option<String>,
    prompt_feedback: Option<GeminiStreamPromptFeedback>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiStreamCandidate {
    content: Option<GeminiStreamContent>,
    finish_reason: Option<String>,
}

#[derive(Deserialize)]
struct GeminiStreamContent {
    #[serde(default)]
    parts: Vec<GeminiStreamPart>,
}

#[derive(Deserialize)]
struct GeminiStreamPart {
    #[serde(default)]
    text: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiStreamUsageMetadata {
    prompt_token_count: u32,
    candidates_token_count: u32,
    total_token_count: u32,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiStreamPromptFeedback {
    block_reason: Option<String>,
}

#[derive(Deserialize)]
struct GeminiErrorResponse {
    error: GeminiErrorDetail,
}

#[derive(Deserialize)]
struct GeminiErrorDetail {
    message: String,
}

/// Executes a streaming chat completion request against Google's Generative Language
/// API (`POST {base_url}/v1beta/models/{model}:streamGenerateContent?alt=sse`).
///
/// The request body is built by calling the existing [`to_gemini_request`] mapping
/// function unchanged — Gemini's streaming variant needs no body flag, only the
/// different path segment (`:streamGenerateContent` instead of `:generateContent`) and
/// the required `alt=sse` query parameter (without it Gemini returns a JSON array
/// instead of an SSE stream).
///
/// Each event's `responseId` (Gemini's documented, stable per-response identifier) is
/// preferred as the yielded chunk's `id` when present. If a streaming response never
/// carries one, this adapter falls back to generating an `id` via `uuid::Uuid::new_v4()`
/// the first time a chunk is yielded and reuses it for the rest of the stream,
/// mirroring `from_gemini_response`'s non-streaming fallback for a missing
/// `responseId`.
///
/// # Arguments
/// * `provider` — The `GeminiProvider` holding the HTTP client, base URL, `KeyStore`,
///   and retry policy.
/// * `req` — Completion request to stream.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `req.messages` contains no non-system
/// message (see `to_gemini_request`), `DomainError::KeyNotFound` if the API key
/// cannot be resolved via `KeyStore`, `DomainError::Timeout` on a connection/request
/// timeout establishing the stream, and `DomainError::ProviderError` for any other
/// transport failure or non-2xx initial response. Once the stream has started, a
/// malformed SSE event, a `promptFeedback.blockReason` (safety block), or a
/// stream-body read failure is surfaced as an `Err(DomainError::ProviderError)` *item*
/// within the stream rather than as a top-level `Err` from this function.
pub(super) fn stream<'a>(
    provider: &'a GeminiProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>> {
    let body = to_gemini_request(req);
    let model = req.model.clone();
    let url = format!(
        "{}/v1beta/models/{model}:streamGenerateContent?alt=sse",
        provider.base_url
    );

    Box::pin(async move {
        let body = body?;
        let api_key = provider.key_store.get_key(GeminiProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("x-goog-api-key", api_key.as_str())
                .json(&body)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source("streaming request to Gemini failed", e)
                }
            })?;

        if response.status() == reqwest::StatusCode::TOO_MANY_REQUESTS {
            let retry_after_secs = response
                .headers()
                .get(reqwest::header::RETRY_AFTER)
                .and_then(|v| v.to_str().ok())
                .and_then(|v| v.parse::<u64>().ok());
            return Err(DomainError::RateLimited { retry_after_secs });
        }

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let message = serde_json::from_str::<GeminiErrorResponse>(&body)
                .map(|e| e.error.message)
                .unwrap_or(body);
            return Err(DomainError::provider_error(format!(
                "Gemini API error ({status}): {message}"
            )));
        }

        let event_stream = response.bytes_stream().eventsource();

        let chunks = try_stream! {
            // Seeded from the first event that carries a `responseId`, and reused for
            // the rest of the stream. Falls back to a lazily-generated UUID only if no
            // event ever carries one.
            let mut response_id: Option<String> = None;

            for await event in event_stream {
                let event = event.map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to read Gemini SSE event",
                        e,
                    )
                })?;

                if event.data.is_empty() {
                    continue;
                }

                let parsed: GeminiStreamEvent = serde_json::from_str(&event.data).map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to parse Gemini streaming event",
                        e,
                    )
                })?;

                if let Some(block_reason) = parsed
                    .prompt_feedback
                    .as_ref()
                    .and_then(|f| f.block_reason.as_ref())
                {
                    Err(DomainError::provider_error(format!(
                        "Gemini blocked the request (reason: {block_reason})"
                    )))?;
                }

                let id = match parsed.response_id.clone() {
                    Some(rid) => {
                        response_id = Some(rid.clone());
                        rid
                    }
                    None => response_id
                        .get_or_insert_with(|| uuid::Uuid::new_v4().to_string())
                        .clone(),
                };

                let candidate = parsed.candidates.into_iter().next();

                let delta = candidate.as_ref().and_then(|c| {
                    c.content.as_ref().map(|content| {
                        content
                            .parts
                            .iter()
                            .map(|p| p.text.as_str())
                            .collect::<Vec<_>>()
                            .join("")
                    })
                });

                let finish_reason = candidate.and_then(|c| c.finish_reason);

                let usage = parsed.usage_metadata.map(|u| Usage {
                    prompt_tokens: u.prompt_token_count,
                    completion_tokens: u.candidates_token_count,
                    total_tokens: u.total_token_count,
                });

                yield CompletionChunk {
                    id,
                    model: model.clone(),
                    delta,
                    finish_reason,
                    usage,
                };
            }
        };

        Ok(Box::pin(chunks) as BoxStream<'static, Result<CompletionChunk, DomainError>>)
    })
}
