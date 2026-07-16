use async_stream::try_stream;
use eventsource_stream::Eventsource;
use futures::future::BoxFuture;
use futures::stream::BoxStream;
use serde::Deserialize;

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, Usage};

use super::request::to_anthropic_request;
use super::{ANTHROPIC_VERSION, AnthropicProvider};

/// Discriminant of a single Anthropic Messages API streaming SSE event, read from the
/// `"type"` field embedded in the event's `data` JSON payload.
///
/// Anthropic sets the SSE frame's own `event:` line to the same value, but this adapter
/// dispatches on `data.type` instead: `eventsource-stream` already hands back the raw
/// `data` string that must be parsed as JSON regardless, so reading the discriminant
/// from the same parsed value avoids a second, redundant parse of the `event:` line and
/// keeps a single source of truth if the two ever diverge.
#[derive(Deserialize)]
#[serde(tag = "type")]
#[serde(rename_all = "snake_case")]
enum AnthropicStreamEvent {
    MessageStart {
        message: AnthropicStreamMessageStart,
    },
    ContentBlockStart,
    ContentBlockDelta {
        delta: AnthropicStreamDelta,
    },
    ContentBlockStop,
    MessageDelta {
        delta: AnthropicStreamMessageDelta,
        usage: AnthropicStreamDeltaUsage,
    },
    MessageStop,
    Ping,
    Error {
        error: AnthropicStreamError,
    },
}

#[derive(Deserialize)]
struct AnthropicStreamMessageStart {
    id: String,
    model: String,
    usage: AnthropicStreamStartUsage,
}

#[derive(Deserialize)]
struct AnthropicStreamStartUsage {
    input_tokens: u32,
}

#[derive(Deserialize)]
#[serde(tag = "type")]
#[serde(rename_all = "snake_case")]
enum AnthropicStreamDelta {
    TextDelta {
        text: String,
    },
    #[serde(other)]
    Other,
}

#[derive(Deserialize)]
struct AnthropicStreamMessageDelta {
    stop_reason: Option<String>,
}

#[derive(Deserialize)]
struct AnthropicStreamDeltaUsage {
    output_tokens: u32,
}

#[derive(Deserialize)]
struct AnthropicStreamError {
    message: String,
}

#[derive(Deserialize)]
struct AnthropicErrorResponse {
    error: AnthropicErrorDetail,
}

#[derive(Deserialize)]
struct AnthropicErrorDetail {
    message: String,
}

/// Running state accumulated across an Anthropic streaming response, captured from
/// `message_start` and reused on every subsequent `CompletionChunk` this adapter yields
/// (Anthropic's later event types do not repeat the response `id`/`model`).
#[derive(Default)]
struct StreamState {
    id: String,
    model: String,
    input_tokens: u32,
}

/// Executes a streaming chat completion request against the Anthropic Messages API
/// (`POST {base_url}/v1/messages` with `"stream": true`).
///
/// The request body is built by calling the existing (non-streaming)
/// [`to_anthropic_request`] mapping function, serializing it to a `serde_json::Value`,
/// and injecting `"stream": true` — this reuses the shared request-mapping logic
/// without adding a `stream` field to `AnthropicRequest` itself.
///
/// Design choices (see Step 43 scope):
/// - `message_start` is not itself yielded as a `CompletionChunk`; it only seeds the
///   running [`StreamState`] (`id`, `model`, `input_tokens`) used by later chunks.
/// - `content_block_delta` events with a `text_delta` are yielded as incremental
///   `CompletionChunk`s carrying only `delta`. Any other delta type (e.g. a future
///   tool-use `input_json_delta`) is skipped with a `tracing::warn!`, since tool-call
///   streaming is out of scope for this step.
/// - `message_delta` yields exactly one final `CompletionChunk` carrying
///   `finish_reason` and the combined `usage` (`input_tokens` from `message_start` +
///   `output_tokens` from this event).
/// - `message_stop` ends the stream (no chunk). `content_block_start`/
///   `content_block_stop`/`ping` are ignored. An `error` event yields a single `Err`
///   item and ends the stream.
///
/// # Arguments
/// * `provider` — The `AnthropicProvider` holding the HTTP client, base URL,
///   `KeyStore`, and retry policy.
/// * `req` — Completion request to stream.
///
/// # Errors
/// Returns `DomainError::KeyNotFound` if the API key cannot be resolved via `KeyStore`,
/// `DomainError::Timeout` on a connection/request timeout establishing the stream, and
/// `DomainError::ProviderError` for any other transport failure or non-2xx initial
/// response. Once the stream has started, a malformed SSE event, an Anthropic `error`
/// event, or a stream-body read failure is surfaced as an
/// `Err(DomainError::ProviderError)` *item* within the stream rather than as a top-level
/// `Err` from this function.
pub(super) fn stream<'a>(
    provider: &'a AnthropicProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>> {
    let mut body = serde_json::to_value(to_anthropic_request(req)).unwrap_or_else(|e| {
        // `to_anthropic_request`'s output always serializes successfully; this branch
        // exists only to avoid a `panic!` if that ever stops being true.
        tracing::error!(error = %e, "failed to serialize Anthropic streaming request body");
        serde_json::json!({})
    });
    if let Some(obj) = body.as_object_mut() {
        obj.insert("stream".to_string(), serde_json::json!(true));
    }
    let url = format!("{}/v1/messages", provider.base_url);

    Box::pin(async move {
        let api_key = provider
            .key_store
            .get_key(AnthropicProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("x-api-key", &api_key)
                .header("anthropic-version", ANTHROPIC_VERSION)
                .json(&body)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source(
                        "streaming request to Anthropic failed",
                        e,
                    )
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
            let message = serde_json::from_str::<AnthropicErrorResponse>(&body)
                .map(|e| e.error.message)
                .unwrap_or(body);
            return Err(DomainError::provider_error(format!(
                "Anthropic API error ({status}): {message}"
            )));
        }

        let event_stream = response.bytes_stream().eventsource();

        let chunks = try_stream! {
            let mut state = StreamState::default();

            for await event in event_stream {
                let event = event.map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to read Anthropic SSE event",
                        e,
                    )
                })?;

                // Anthropic also sends a `ping` control frame with an empty/irrelevant
                // body between real events; an empty `data` is not valid JSON, so skip
                // it before attempting to parse.
                if event.data.is_empty() {
                    continue;
                }

                let parsed: AnthropicStreamEvent = serde_json::from_str(&event.data).map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to parse Anthropic streaming event",
                        e,
                    )
                })?;

                match parsed {
                    AnthropicStreamEvent::MessageStart { message } => {
                        state.id = message.id;
                        state.model = message.model;
                        state.input_tokens = message.usage.input_tokens;
                    }
                    AnthropicStreamEvent::ContentBlockDelta { delta } => match delta {
                        AnthropicStreamDelta::TextDelta { text } => {
                            yield CompletionChunk {
                                id: state.id.clone(),
                                model: state.model.clone(),
                                delta: Some(text),
                                finish_reason: None,
                                usage: None,
                            };
                        }
                        AnthropicStreamDelta::Other => {
                            tracing::warn!(
                                "skipping unsupported Anthropic content_block_delta type \
                                 (e.g. tool-use input_json_delta)"
                            );
                        }
                    },
                    AnthropicStreamEvent::MessageDelta { delta, usage } => {
                        yield CompletionChunk {
                            id: state.id.clone(),
                            model: state.model.clone(),
                            delta: None,
                            finish_reason: delta.stop_reason,
                            usage: Some(Usage {
                                prompt_tokens: state.input_tokens,
                                completion_tokens: usage.output_tokens,
                                total_tokens: state.input_tokens + usage.output_tokens,
                            }),
                        };
                    }
                    AnthropicStreamEvent::MessageStop => break,
                    AnthropicStreamEvent::Error { error } => {
                        Err(DomainError::provider_error(format!(
                            "Anthropic streaming error: {}",
                            error.message
                        )))?;
                    }
                    AnthropicStreamEvent::ContentBlockStart
                    | AnthropicStreamEvent::ContentBlockStop
                    | AnthropicStreamEvent::Ping => {}
                }
            }
        };

        Ok(Box::pin(chunks) as BoxStream<'static, Result<CompletionChunk, DomainError>>)
    })
}
