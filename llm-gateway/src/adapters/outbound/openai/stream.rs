use async_stream::try_stream;
use eventsource_stream::Eventsource;
use futures::future::BoxFuture;
use futures::stream::BoxStream;
use serde::Deserialize;

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, Usage};

use super::OpenAIProvider;
use super::request::to_openai_request;

/// Literal SSE payload OpenAI sends as the final `data:` line of a streaming Chat
/// Completions response, signalling that no further events follow.
const DONE_SENTINEL: &str = "[DONE]";

/// Shape of a single OpenAI streaming Chat Completions SSE event's `data` payload.
///
/// Mirrors the non-streaming `OpenAIResponse` in `request.rs`, but `choices[].delta`
/// replaces `choices[].message` (incremental content instead of a full message), and
/// `usage` is only present on the final event when the request opts in via
/// `stream_options.include_usage` (which this adapter always sets).
#[derive(Deserialize)]
struct OpenAIStreamEvent {
    id: String,
    model: String,
    #[serde(default)]
    choices: Vec<OpenAIStreamChoice>,
    usage: Option<OpenAIStreamUsage>,
}

#[derive(Deserialize)]
struct OpenAIStreamChoice {
    delta: OpenAIStreamDelta,
    finish_reason: Option<String>,
}

#[derive(Deserialize, Default)]
struct OpenAIStreamDelta {
    content: Option<String>,
}

#[derive(Deserialize)]
struct OpenAIStreamUsage {
    prompt_tokens: u32,
    completion_tokens: u32,
    total_tokens: u32,
}

#[derive(Deserialize)]
struct OpenAIErrorResponse {
    error: OpenAIErrorDetail,
}

#[derive(Deserialize)]
struct OpenAIErrorDetail {
    message: String,
}

/// Executes a streaming chat completion request against the OpenAI Chat Completions API
/// (`POST {base_url}/v1/chat/completions` with `"stream": true`).
///
/// The request body is built by calling the existing (non-streaming) [`to_openai_request`]
/// mapping function, serializing it to a `serde_json::Value`, and injecting `"stream":
/// true` plus `"stream_options": {"include_usage": true}` — this reuses the shared
/// request-mapping logic without adding a `stream` field to `OpenAIRequest` itself.
///
/// The initial connection attempt reuses [`send_with_retry`] and the same
/// timeout/network-error mapping `request::complete` applies. Once a `200 OK` streaming
/// response is established, each parsed SSE event (via `eventsource-stream`'s
/// `Eventsource` adapter) is decoded into a [`CompletionChunk`]; the literal `data:
/// [DONE]` sentinel line ends the stream without being parsed as JSON. A malformed event
/// body yields a single `Err` item and ends the stream, rather than panicking.
///
/// # Arguments
/// * `provider` — The `OpenAIProvider` holding the HTTP client, base URL, `KeyStore`,
///   and retry policy.
/// * `req` — Completion request to stream.
///
/// # Errors
/// Returns `DomainError::KeyNotFound` if the API key cannot be resolved via `KeyStore`,
/// `DomainError::Timeout` on a connection/request timeout establishing the stream, and
/// `DomainError::ProviderError` for any other transport failure or non-2xx initial
/// response (surfaced before the returned stream is even constructed). Once the stream
/// has started, a malformed SSE event or a stream-body read failure is surfaced as an
/// `Err(DomainError::ProviderError)` *item* within the stream rather than as a top-level
/// `Err` from this function.
pub(super) fn stream<'a>(
    provider: &'a OpenAIProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>> {
    let mut body = serde_json::to_value(to_openai_request(req)).unwrap_or_else(|e| {
        // `to_openai_request`'s output always serializes successfully (it is built
        // entirely from `String`/`Option<_>`/`Vec<_>` fields); this branch exists only
        // to avoid a `panic!` if that ever stops being true.
        tracing::error!(error = %e, "failed to serialize OpenAI streaming request body");
        serde_json::json!({})
    });
    if let Some(obj) = body.as_object_mut() {
        obj.insert("stream".to_string(), serde_json::json!(true));
        obj.insert(
            "stream_options".to_string(),
            serde_json::json!({"include_usage": true}),
        );
    }
    let url = format!("{}/v1/chat/completions", provider.base_url);

    Box::pin(async move {
        let api_key = provider.key_store.get_key(OpenAIProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("Authorization", format!("Bearer {api_key}"))
                .json(&body)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source("streaming request to OpenAI failed", e)
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
            let message = serde_json::from_str::<OpenAIErrorResponse>(&body)
                .map(|e| e.error.message)
                .unwrap_or(body);
            return Err(DomainError::provider_error(format!(
                "OpenAI API error ({status}): {message}"
            )));
        }

        let event_stream = response.bytes_stream().eventsource();

        let chunks = try_stream! {
            for await event in event_stream {
                let event = event.map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to read OpenAI SSE event",
                        e,
                    )
                })?;

                if event.data == DONE_SENTINEL {
                    break;
                }

                let parsed: OpenAIStreamEvent = serde_json::from_str(&event.data).map_err(|e| {
                    DomainError::provider_error_with_source(
                        "failed to parse OpenAI streaming event",
                        e,
                    )
                })?;

                let (delta, finish_reason) = match parsed.choices.into_iter().next() {
                    Some(choice) => (choice.delta.content, choice.finish_reason),
                    None => (None, None),
                };

                yield CompletionChunk {
                    id: parsed.id,
                    model: parsed.model,
                    delta,
                    finish_reason,
                    usage: parsed.usage.map(|u| Usage {
                        prompt_tokens: u.prompt_tokens,
                        completion_tokens: u.completion_tokens,
                        total_tokens: u.total_tokens,
                    }),
                };
            }
        };

        Ok(Box::pin(chunks) as BoxStream<'static, Result<CompletionChunk, DomainError>>)
    })
}
