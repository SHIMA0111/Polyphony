//! `POST /completions/stream` — Server-Sent Events (SSE) streaming completion endpoint.
//!
//! Exposes the same request shape as `POST /completions` (see `rest::request`), but
//! calls `CompletionUseCase::stream` instead of `CompletionUseCase::complete` and
//! re-emits the resulting `CompletionChunk`s as an outbound `text/event-stream`
//! response, one JSON-encoded [`CompletionChunkDto`] per SSE `data:` frame, uniformly
//! across all three providers (OpenAI, Anthropic, Gemini) regardless of which one
//! served the request.
//!
//! # SSE contract
//! - Each successfully produced chunk is emitted as a `data: {...}` event carrying a
//!   JSON-encoded [`CompletionChunkDto`].
//! - A mid-stream error (the upstream provider stream yielded an `Err` item) is emitted
//!   as an `event: error` frame carrying the error's `Display` message as plain text.
//!   The overall HTTP response status stays `200 OK` in this case, since the status
//!   line and headers were already committed before the error occurred — there is no
//!   way to retroactively surface it as a `4xx`/`5xx`.
//! - The stream always ends with a final literal `data: [DONE]` event, mirroring
//!   OpenAI's own streaming convention, applied uniformly so Step 51 (Go: consume
//!   gateway stream + WS chunk forwarding) has one contract to consume regardless of
//!   provider.
//!
//! A pre-stream error (e.g. `DomainError::ModelNotFound` because `CompletionUseCase::
//! stream` itself fails before any bytes are sent) is *not* covered by the contract
//! above: it still surfaces as an ordinary non-SSE HTTP error response via the existing
//! [`AppError`] status mapping, exactly like `POST /completions`.

use std::convert::Infallible;

use axum::Json;
use axum::extract::State;
use axum::response::Sse;
use axum::response::sse::{Event, KeepAlive};
use futures::{Stream, StreamExt};
use serde::Serialize;

use crate::domain::model::CompletionChunk;

use super::handlers::{AppError, AppState};
use super::request::CompletionRequestDto;

/// Streaming completion chunk DTO for the REST API.
///
/// Defined locally in this file (rather than in `rest::response`) so that
/// `rest/response.rs` — owned by the same-wave Vision step — is never touched by this
/// step. Deliberately mirrors `CompletionChunk`'s shape field-for-field.
#[derive(Serialize)]
pub struct CompletionChunkDto {
    pub id: String,
    pub model: String,
    /// Incremental text content produced since the previous chunk, if any. Omitted
    /// from the JSON body (rather than serialized as `null`) when absent.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delta: Option<String>,
    /// Set on the final chunk to indicate why generation stopped. Omitted from the
    /// JSON body when absent.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub finish_reason: Option<String>,
    /// Token usage, typically only populated on the final chunk. Omitted from the
    /// JSON body when absent.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub usage: Option<UsageDto>,
}

/// Token usage DTO for a streaming completion chunk.
///
/// A small local counterpart to `rest::response::UsageDto` (not reused directly, so
/// this file never needs to import from `response.rs` and stays independent of that
/// same-wave-owned file).
#[derive(Serialize)]
pub struct UsageDto {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

impl From<CompletionChunk> for CompletionChunkDto {
    fn from(chunk: CompletionChunk) -> Self {
        Self {
            id: chunk.id,
            model: chunk.model,
            delta: chunk.delta,
            finish_reason: chunk.finish_reason,
            usage: chunk.usage.map(|u| UsageDto {
                prompt_tokens: u.prompt_tokens,
                completion_tokens: u.completion_tokens,
                total_tokens: u.total_tokens,
            }),
        }
    }
}

/// Streaming chat completion endpoint.
///
/// `POST /completions/stream` — Sends a chat completion request to an LLM provider and
/// streams the response back as Server-Sent Events. See the module-level docs for the
/// exact SSE contract (one `data:` JSON chunk per event, `event: error` frames for
/// mid-stream failures, a terminating `data: [DONE]` sentinel).
///
/// # Arguments
/// * `service` — Shared `CompletionUseCase` state.
/// * `dto` — Same request shape as `POST /completions`.
///
/// # Returns
/// `200` with a `text/event-stream` body: a `data:` JSON [`CompletionChunkDto`] per
/// chunk, `event: error` frames for mid-stream failures, terminated by a literal
/// `data: [DONE]` event. See the module-level docs for the full SSE contract.
///
/// # Errors
/// Returns `AppError` (mapped to the existing non-SSE HTTP status codes) if the DTO
/// fails to convert to a domain request, or if `CompletionUseCase::stream` itself fails
/// before any chunk is produced (e.g. `DomainError::ModelNotFound`). Once the SSE
/// response has started, subsequent errors are represented as `event: error` frames
/// within the stream body instead of a different HTTP status, since the status/headers
/// are already committed by that point.
pub async fn complete_stream(
    State(service): State<AppState>,
    Json(dto): Json<CompletionRequestDto>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, AppError> {
    let req = dto.into_domain()?;
    let chunk_stream = service.stream(req).await?;

    let events = chunk_stream.map(|item| {
        let event = match item {
            Ok(chunk) => {
                let model = chunk.model.clone();
                Event::default()
                    .json_data(CompletionChunkDto::from(chunk))
                    .unwrap_or_else(|e| {
                        tracing::error!(
                            error = %e,
                            model = %model,
                            "failed to JSON-encode completion chunk for SSE"
                        );
                        Event::default()
                            .event("error")
                            .data(format!("failed to encode completion chunk: {e}"))
                    })
            }
            Err(err) => {
                tracing::error!(error = %err, "upstream provider stream yielded an error");
                Event::default().event("error").data(err.to_string())
            }
        };
        Ok(event)
    });

    let done = futures::stream::once(async { Ok(Event::default().data("[DONE]")) });

    Ok(Sse::new(events.chain(done)).keep_alive(KeepAlive::default()))
}
