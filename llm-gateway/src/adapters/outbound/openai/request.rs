use futures::future::BoxFuture;
use serde::{Deserialize, Serialize};

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, ContentPart, MessageContent, Role,
    Usage,
};

use super::OpenAIProvider;

// --- OpenAI-specific DTOs ---

/// An outbound OpenAI Chat Completions API request body, produced from a domain
/// `CompletionRequest` by [`to_openai_request`].
#[derive(Serialize)]
pub(super) struct OpenAIRequest {
    model: String,
    messages: Vec<OpenAIRequestMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f32>,
    /// Serialized as `max_completion_tokens`, the field name required by newer
    /// Chat Completions models (e.g. `o3`, `o4-mini`), which reject the legacy
    /// `max_tokens` name. OpenAI accepts this name for all current chat models.
    #[serde(
        skip_serializing_if = "Option::is_none",
        rename = "max_completion_tokens"
    )]
    max_tokens: Option<u32>,
}

/// An outbound OpenAI Chat Completions message. `content` is serialized as either a
/// bare string (text-only, the common case) or an array of typed content parts
/// (Vision), matching OpenAI's own union shape for this field.
#[derive(Serialize)]
struct OpenAIRequestMessage {
    role: String,
    content: OpenAIContent,
}

/// OpenAI's `content` union: a plain string for text-only messages, or an array of
/// `OpenAIContentPart`s for multimodal (Vision) messages. `#[serde(untagged)]`
/// serializes `Text` as a bare JSON string and `Parts` as a JSON array, matching
/// OpenAI's Chat Completions API exactly.
#[derive(Serialize)]
#[serde(untagged)]
enum OpenAIContent {
    Text(String),
    Parts(Vec<OpenAIContentPart>),
}

/// A single entry of OpenAI's multimodal `content` array.
#[derive(Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum OpenAIContentPart {
    Text { text: String },
    ImageUrl { image_url: OpenAIImageUrl },
}

/// Nested `image_url` object of an `OpenAIContentPart::ImageUrl`.
#[derive(Serialize)]
struct OpenAIImageUrl {
    url: String,
}

/// An inbound OpenAI Chat Completions message (used only for parsing responses,
/// which are always plain text — no provider adapter yet echoes image content back).
#[derive(Deserialize)]
struct OpenAIMessage {
    role: String,
    content: String,
}

#[derive(Deserialize)]
struct OpenAIResponse {
    id: String,
    model: String,
    choices: Vec<OpenAIChoice>,
    usage: OpenAIUsage,
}

#[derive(Deserialize)]
struct OpenAIChoice {
    index: u32,
    message: OpenAIMessage,
    finish_reason: String,
}

#[derive(Deserialize)]
struct OpenAIUsage {
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

// --- Domain model <-> OpenAI DTO conversion ---

/// Converts a domain `Role` to an OpenAI Chat Completions API role string.
///
/// This is intentionally *not* unified with `Role::as_str`: it targets OpenAI's own
/// wire convention (`System` → `"developer"`), which differs from the canonical
/// REST-wire form used elsewhere in this gateway.
///
/// # Arguments
/// * `role` — Domain role to convert.
///
/// # Returns
/// - `System` → `"developer"` (OpenAI recommended; `"system"` is internally converted to `"developer"`)
/// - `Tool` → `"tool"` (`"function"` is deprecated)
fn role_to_openai_str(role: &Role) -> &'static str {
    match role {
        Role::System => "developer",
        Role::User => "user",
        Role::Assistant => "assistant",
        Role::Tool => "tool",
    }
}

/// Converts a domain `CompletionRequest` into an [`OpenAIRequest`] wire body.
///
/// # Arguments
/// * `req` — Completion request to convert.
///
/// # Returns
/// An `OpenAIRequest` with `model`/`temperature`/`max_tokens` copied over verbatim,
/// `messages` mapped via [`role_to_openai_str`] and [`to_openai_content`] (never
/// fails; see `to_openai_content`'s `# Errors`).
///
/// Infallible: every domain `Role` and `MessageContent`/`ContentPart` variant has a
/// representable OpenAI counterpart, so this function never fails and returns
/// `OpenAIRequest` directly rather than a `Result`.
pub(super) fn to_openai_request(req: &CompletionRequest) -> OpenAIRequest {
    OpenAIRequest {
        model: req.model.clone(),
        messages: req
            .messages
            .iter()
            .map(|m| OpenAIRequestMessage {
                role: role_to_openai_str(&m.role).to_string(),
                content: to_openai_content(&m.content),
            })
            .collect(),
        temperature: req.temperature,
        max_tokens: req.max_tokens,
    }
}

/// Converts a domain `MessageContent` into OpenAI's `content` union shape.
///
/// `MessageContent::Text` serializes as a bare JSON string (OpenAI's original,
/// text-only shape); `MessageContent::Parts` serializes as an array of typed content
/// parts, one entry per `ContentPart` (see `to_openai_content_part`).
///
/// # Errors
/// Never fails: every `ContentPart` variant has a representable OpenAI counterpart
/// (see `to_openai_content_part`).
fn to_openai_content(content: &MessageContent) -> OpenAIContent {
    match content {
        MessageContent::Text(s) => OpenAIContent::Text(s.clone()),
        MessageContent::Parts(parts) => {
            OpenAIContent::Parts(parts.iter().map(to_openai_content_part).collect())
        }
    }
}

/// Converts a single domain `ContentPart` into OpenAI's content-part shape.
///
/// `ContentPart::ImageBase64` has no dedicated OpenAI part type; it is encoded as an
/// `image_url` part whose URL is a `data:{media_type};base64,{data}` data URL, which
/// the OpenAI Chat Completions API accepts as equivalent to a hosted image URL.
fn to_openai_content_part(part: &ContentPart) -> OpenAIContentPart {
    match part {
        ContentPart::Text(text) => OpenAIContentPart::Text { text: text.clone() },
        ContentPart::ImageUrl(url) => OpenAIContentPart::ImageUrl {
            image_url: OpenAIImageUrl { url: url.clone() },
        },
        ContentPart::ImageBase64 { media_type, data } => OpenAIContentPart::ImageUrl {
            image_url: OpenAIImageUrl {
                url: format!("data:{media_type};base64,{data}"),
            },
        },
    }
}

fn from_openai_response(resp: OpenAIResponse) -> CompletionResponse {
    CompletionResponse {
        id: resp.id,
        model: resp.model,
        choices: resp
            .choices
            .into_iter()
            .map(|c| Choice {
                index: c.index,
                message: ChatMessage {
                    role: Role::parse(&c.message.role).unwrap_or_else(|_| {
                        tracing::warn!(
                            role = c.message.role.as_str(),
                            "unknown OpenAI role, falling back to User"
                        );
                        Role::User
                    }),
                    content: c.message.content.into(),
                },
                finish_reason: c.finish_reason,
            })
            .collect(),
        usage: Usage {
            prompt_tokens: resp.usage.prompt_tokens,
            completion_tokens: resp.usage.completion_tokens,
            total_tokens: resp.usage.total_tokens,
        },
    }
}

/// Executes a chat completion request against the OpenAI Chat Completions API.
///
/// The outbound request is retried with bounded exponential backoff on `429`/`5xx`
/// responses via `send_with_retry`; a `429` that persists after retries are exhausted
/// is mapped to `DomainError::RateLimited` and a `5xx` to `DomainError::ProviderError`,
/// same as a non-retryable failure.
///
/// # Arguments
/// * `provider` — The `OpenAIProvider` holding the HTTP client, base URL, `KeyStore`,
///   and retry policy.
/// * `req` — Completion request to send.
///
/// # Errors
/// Returns `DomainError::KeyNotFound` if the API key cannot be resolved via
/// `KeyStore`, `DomainError::Timeout` on a connection/request timeout,
/// `DomainError::RateLimited` on an HTTP 429 response (with `Retry-After` parsed if present),
/// and `DomainError::ProviderError` (with the original error preserved via `#[source]`
/// where available) for any other transport or non-2xx response.
pub(super) fn complete<'a>(
    provider: &'a OpenAIProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<CompletionResponse, DomainError>> {
    let openai_req = to_openai_request(req);
    let url = format!("{}/v1/chat/completions", provider.base_url);

    Box::pin(async move {
        let api_key = provider.key_store.get_key(OpenAIProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("Authorization", format!("Bearer {api_key}"))
                .json(&openai_req)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source("request to OpenAI failed", e)
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

        let openai_resp: OpenAIResponse = response.json().await.map_err(|e| {
            DomainError::provider_error_with_source("failed to parse OpenAI response", e)
        })?;

        Ok(from_openai_response(openai_resp))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::ChatMessage;

    #[test]
    fn test_to_openai_request() {
        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: "You are helpful.".to_string().into(),
                },
                ChatMessage {
                    role: Role::User,
                    content: "Hello".to_string().into(),
                },
            ],
            temperature: Some(0.7),
            max_tokens: Some(1000),
        };

        let openai_req = to_openai_request(&req);
        assert_eq!(openai_req.model, "gpt-5.2");
        assert_eq!(openai_req.messages.len(), 2);
        assert_eq!(openai_req.messages[0].role, "developer");
        assert_eq!(openai_req.messages[1].role, "user");
        assert_eq!(openai_req.temperature, Some(0.7));
        assert_eq!(openai_req.max_tokens, Some(1000));
    }

    /// The token limit must serialize on the wire as `max_completion_tokens`, not the
    /// legacy `max_tokens` name — `o3`/`o4-mini` and other modern Chat Completions
    /// models reject `max_tokens` outright.
    #[test]
    fn test_to_openai_request_serializes_max_completion_tokens() {
        let req = CompletionRequest {
            model: "o4-mini".to_string(),
            messages: vec![ChatMessage {
                role: Role::User,
                content: "Hello".to_string().into(),
            }],
            temperature: None,
            max_tokens: Some(500),
        };

        let json = serde_json::to_value(to_openai_request(&req)).unwrap();
        assert_eq!(json["max_completion_tokens"], serde_json::json!(500));
        assert!(
            json.get("max_tokens").is_none(),
            "expected no legacy `max_tokens` key, got: {json}"
        );
    }

    /// `MessageContent::Text` keeps serializing as a bare JSON string, not a
    /// single-element array — this is the backward-compatibility guarantee for
    /// existing text-only clients.
    #[test]
    fn test_to_openai_content_text_serializes_as_bare_string() {
        let content = to_openai_content(&MessageContent::Text("hello".to_string()));
        assert_eq!(
            serde_json::to_value(&content).unwrap(),
            serde_json::json!("hello")
        );
    }

    /// `MessageContent::Parts` serializes as an array of typed parts; `ImageBase64`
    /// becomes an `image_url` part whose URL is a `data:` URL, since OpenAI has no
    /// dedicated inline-base64 part type.
    #[test]
    fn test_to_openai_content_parts_serializes_as_array_with_data_url_for_base64() {
        let content = MessageContent::Parts(vec![
            ContentPart::Text("look: ".to_string()),
            ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
            ContentPart::ImageBase64 {
                media_type: "image/png".to_string(),
                data: "abcd".to_string(),
            },
        ]);

        let json = serde_json::to_value(to_openai_content(&content)).unwrap();
        assert_eq!(
            json,
            serde_json::json!([
                {"type": "text", "text": "look: "},
                {"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,abcd"}},
            ])
        );
    }

    /// End-to-end: a `CompletionRequest` with a mixed text+image message serializes to
    /// the exact OpenAI request body shape, asserted at the full-request level (not
    /// just the isolated content mapping).
    #[test]
    fn test_to_openai_request_with_image_parts_serializes_full_message_body() {
        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![ChatMessage {
                role: Role::User,
                content: MessageContent::Parts(vec![
                    ContentPart::Text("what is this?".to_string()),
                    ContentPart::ImageBase64 {
                        media_type: "image/png".to_string(),
                        data: "abcd".to_string(),
                    },
                ]),
            }],
            temperature: None,
            max_tokens: None,
        };

        let openai_req = to_openai_request(&req);
        let json = serde_json::to_value(&openai_req).unwrap();
        assert_eq!(
            json["messages"][0]["content"],
            serde_json::json!([
                {"type": "text", "text": "what is this?"},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,abcd"}},
            ])
        );
    }

    #[test]
    fn test_from_openai_response() {
        let openai_resp = OpenAIResponse {
            id: "chatcmpl-123".to_string(),
            model: "gpt-5.2".to_string(),
            choices: vec![OpenAIChoice {
                index: 0,
                message: OpenAIMessage {
                    role: "assistant".to_string(),
                    content: "Hi there!".to_string(),
                },
                finish_reason: "stop".to_string(),
            }],
            usage: OpenAIUsage {
                prompt_tokens: 20,
                completion_tokens: 5,
                total_tokens: 25,
            },
        };

        let resp = from_openai_response(openai_resp);
        assert_eq!(resp.id, "chatcmpl-123");
        assert_eq!(resp.model, "gpt-5.2");
        assert_eq!(resp.choices.len(), 1);
        assert_eq!(resp.choices[0].message.role, Role::Assistant);
        assert_eq!(resp.choices[0].message.content.as_text(), "Hi there!");
        assert_eq!(resp.usage.prompt_tokens, 20);
        assert_eq!(resp.usage.total_tokens, 25);
    }

    #[test]
    fn test_role_to_openai_str() {
        assert_eq!(role_to_openai_str(&Role::System), "developer");
        assert_eq!(role_to_openai_str(&Role::User), "user");
        assert_eq!(role_to_openai_str(&Role::Assistant), "assistant");
        assert_eq!(role_to_openai_str(&Role::Tool), "tool");
    }

    /// `from_openai_response` falls back to `Role::User` (with a warning) for an
    /// unparseable role string, preserving the adapter's previous lenient behavior,
    /// now implemented via the shared `Role::parse`.
    #[test]
    fn test_from_openai_response_unknown_role_falls_back_to_user() {
        let openai_resp = OpenAIResponse {
            id: "chatcmpl-456".to_string(),
            model: "gpt-5.2".to_string(),
            choices: vec![OpenAIChoice {
                index: 0,
                message: OpenAIMessage {
                    role: "totally-unknown-role".to_string(),
                    content: "hi".to_string(),
                },
                finish_reason: "stop".to_string(),
            }],
            usage: OpenAIUsage {
                prompt_tokens: 1,
                completion_tokens: 1,
                total_tokens: 2,
            },
        };

        let resp = from_openai_response(openai_resp);
        assert_eq!(resp.choices[0].message.role, Role::User);
    }
}
