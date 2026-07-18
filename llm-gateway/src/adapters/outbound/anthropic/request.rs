use futures::future::BoxFuture;
use serde::{Deserialize, Serialize};

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::adapters::outbound::system_message_text;
use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, ContentPart, MessageContent, Role,
    Usage,
};

use super::{ANTHROPIC_VERSION, AnthropicProvider};

/// Default `max_tokens` sent to the Anthropic Messages API when the domain
/// `CompletionRequest.max_tokens` is `None`.
///
/// Unlike OpenAI's Chat Completions API, Anthropic's Messages API requires
/// `max_tokens` on every request and returns a `400 invalid_request_error` if it is
/// omitted, so this adapter must always supply a value.
pub const DEFAULT_MAX_TOKENS: u32 = 4096;

// --- Anthropic-specific DTOs ---

#[derive(Serialize)]
pub(super) struct AnthropicRequest {
    model: String,
    max_tokens: u32,
    messages: Vec<AnthropicMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    system: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f32>,
}

/// An outbound Anthropic Messages API message. `content` is serialized as either a
/// bare string (text-only) or an array of content blocks (Vision), matching
/// Anthropic's own union shape for this field.
#[derive(Serialize)]
struct AnthropicMessage {
    role: String,
    content: AnthropicContent,
}

/// Anthropic's `content` union: a plain string for text-only messages, or an array of
/// `AnthropicContentBlock`s for multimodal (Vision) messages. `#[serde(untagged)]`
/// serializes `Text` as a bare JSON string and `Blocks` as a JSON array, matching
/// Anthropic's Messages API exactly.
#[derive(Serialize)]
#[serde(untagged)]
enum AnthropicContent {
    Text(String),
    Blocks(Vec<AnthropicContentBlockDto>),
}

/// A single outbound content block of Anthropic's multimodal `content` array.
///
/// Named `*Dto` to disambiguate from `AnthropicContentBlock` below, which is the
/// *inbound* (response-parsing) content-block shape and has a different structure
/// (a flat `{type, text}` pair, since Anthropic's response blocks are simpler than
/// its request blocks).
#[derive(Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum AnthropicContentBlockDto {
    Text { text: String },
    Image { source: AnthropicImageSource },
}

/// The `source` object of an Anthropic `image` content block.
///
/// Anthropic supports both `"base64"` (inline bytes) and `"url"` (hosted image)
/// source types for the same `image` block type; which one is used depends on
/// which domain `ContentPart` variant it was built from (see `to_anthropic_content_block`).
#[derive(Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum AnthropicImageSource {
    Base64 { media_type: String, data: String },
    Url { url: String },
}

#[derive(Deserialize)]
struct AnthropicResponse {
    id: String,
    model: String,
    content: Vec<AnthropicContentBlock>,
    stop_reason: String,
    usage: AnthropicUsage,
}

#[derive(Deserialize)]
struct AnthropicContentBlock {
    #[serde(rename = "type")]
    block_type: String,
    #[serde(default)]
    text: String,
}

#[derive(Deserialize)]
struct AnthropicUsage {
    input_tokens: u32,
    output_tokens: u32,
}

#[derive(Deserialize)]
struct AnthropicErrorResponse {
    error: AnthropicErrorDetail,
}

#[derive(Deserialize)]
struct AnthropicErrorDetail {
    message: String,
}

// --- Domain model <-> Anthropic DTO conversion ---

/// Converts a domain `Role` to an Anthropic Messages API role string.
///
/// # Arguments
/// * `role` — Domain role to convert. `Role::System` is never passed here: system
///   messages are hoisted to the request's top-level `system` field by
///   [`to_anthropic_request`] and excluded from the `messages` array entirely, since
///   Anthropic's Messages API does not accept a `system`-role message inside
///   `messages`.
///
/// # Returns
/// - `User` → `"user"`
/// - `Assistant` → `"assistant"`
/// - `Tool` → `"user"` (documented best-effort fallback: Anthropic's Messages API
///   represents tool results as a dedicated `tool_result` content block on a
///   `user`-role message, not as its own role; full tool-use/tool-result content-block
///   support is not implemented by this adapter, so a `Role::Tool` message is instead
///   sent as plain-text `user` content — a known limitation)
/// - `System` → `"user"` (should not occur in practice; see above)
fn role_to_anthropic_str(role: &Role) -> &'static str {
    match role {
        Role::System => "user",
        Role::User => "user",
        Role::Assistant => "assistant",
        Role::Tool => "user",
    }
}

/// Converts a domain `CompletionRequest` into an `AnthropicRequest`.
///
/// System messages (`Role::System`) are removed from the message list and
/// concatenated (joined with `"\n\n"` when there is more than one) into the
/// top-level `system` field, matching Anthropic's Messages API shape. `max_tokens`
/// is defaulted to [`DEFAULT_MAX_TOKENS`] when the domain request does not specify
/// one, since Anthropic requires it on every request.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` when `req.messages` contains no
/// non-`Role::System` message: after system messages are hoisted into `system`,
/// Anthropic's Messages API requires a non-empty `messages` array, and an all-system
/// request would otherwise be sent with `messages: []` and fail remotely with an
/// opaque `400 invalid_request_error` instead of being rejected locally with a clear
/// message. Also returns `DomainError::InvalidRequest` if any system message's content
/// is `MessageContent::Parts` containing a non-text part (see `system_message_text`):
/// Anthropic's `system` field is a flat string and cannot carry an image.
pub(super) fn to_anthropic_request(req: &CompletionRequest) -> Result<AnthropicRequest, DomainError> {
    let mut system_parts = Vec::new();
    let mut messages = Vec::new();

    for m in &req.messages {
        if m.role == Role::System {
            system_parts.push(system_message_text(&m.content)?);
        } else {
            messages.push(AnthropicMessage {
                role: role_to_anthropic_str(&m.role).to_string(),
                content: to_anthropic_content(&m.content),
            });
        }
    }

    if messages.is_empty() {
        return Err(DomainError::InvalidRequest(
            "messages must contain at least one non-system message".to_string(),
        ));
    }

    let system = if system_parts.is_empty() {
        None
    } else {
        Some(system_parts.join("\n\n"))
    };

    Ok(AnthropicRequest {
        model: req.model.clone(),
        max_tokens: req.max_tokens.unwrap_or(DEFAULT_MAX_TOKENS),
        messages,
        system,
        temperature: req.temperature,
    })
}

/// Converts a domain `MessageContent` into Anthropic's `content` union shape.
///
/// `MessageContent::Text` serializes as a bare JSON string (Anthropic accepts this
/// shorthand for a single text block); `MessageContent::Parts` serializes as an
/// explicit array of content blocks, one entry per `ContentPart` (see
/// `to_anthropic_content_block`).
///
/// # Errors
/// Never fails: every `ContentPart` variant has a representable Anthropic content
/// block (see `to_anthropic_content_block`).
fn to_anthropic_content(content: &MessageContent) -> AnthropicContent {
    match content {
        MessageContent::Text(s) => AnthropicContent::Text(s.clone()),
        MessageContent::Parts(parts) => {
            AnthropicContent::Blocks(parts.iter().map(to_anthropic_content_block).collect())
        }
    }
}

/// Converts a single domain `ContentPart` into an Anthropic content block.
///
/// `ContentPart::ImageBase64` maps to an `image` block with a `"base64"` source,
/// Anthropic's native inline-image shape. `ContentPart::ImageUrl` maps to an `image`
/// block with a `"url"` source: Anthropic's Messages API added the `"url"` image
/// source type as an additive extension to the same `image` block (no
/// `anthropic-version` bump was required for it, unlike some other API changes), so
/// it is safe to pass through directly under [`ANTHROPIC_VERSION`] rather than
/// rejecting it with `DomainError::InvalidRequest` — this is the deliberate choice
/// documented in Step 39's plan for this adapter.
fn to_anthropic_content_block(part: &ContentPart) -> AnthropicContentBlockDto {
    match part {
        ContentPart::Text(text) => AnthropicContentBlockDto::Text { text: text.clone() },
        ContentPart::ImageUrl(url) => AnthropicContentBlockDto::Image {
            source: AnthropicImageSource::Url { url: url.clone() },
        },
        ContentPart::ImageBase64 { media_type, data } => AnthropicContentBlockDto::Image {
            source: AnthropicImageSource::Base64 {
                media_type: media_type.clone(),
                data: data.clone(),
            },
        },
    }
}

/// Converts an `AnthropicResponse` into the shared domain `CompletionResponse`.
///
/// Anthropic's `content` array is concatenated (only `"text"`-type blocks contribute)
/// into a single string, since the domain `Choice.message.content` is not itself
/// multi-block. There is always exactly one `Choice` at `index: 0`, since Anthropic's
/// Messages API is not multi-choice. `stop_reason` is passed straight through as
/// `finish_reason`. `usage.total_tokens` is computed as
/// `input_tokens + output_tokens`, since Anthropic's `usage` object has no
/// `total_tokens` field of its own.
fn from_anthropic_response(resp: AnthropicResponse) -> CompletionResponse {
    let content = resp
        .content
        .into_iter()
        .filter(|b| b.block_type == "text")
        .map(|b| b.text)
        .collect::<Vec<_>>()
        .join("");

    let prompt_tokens = resp.usage.input_tokens;
    let completion_tokens = resp.usage.output_tokens;

    CompletionResponse {
        id: resp.id,
        model: resp.model,
        choices: vec![Choice {
            index: 0,
            message: ChatMessage {
                role: Role::Assistant,
                content: content.into(),
            },
            finish_reason: resp.stop_reason,
        }],
        usage: Usage {
            prompt_tokens,
            completion_tokens,
            total_tokens: prompt_tokens + completion_tokens,
        },
    }
}

/// Executes a chat completion request against the Anthropic Messages API
/// (`POST {base_url}/v1/messages`).
///
/// The outbound request is retried with bounded exponential backoff on `429`/`5xx`
/// responses (including Anthropic's `529 overloaded_error`, which falls within the
/// `5xx` range) via `send_with_retry`; a `429` that persists after retries are
/// exhausted is mapped to `DomainError::RateLimited` and a `5xx` to
/// `DomainError::ProviderError`, same as a non-retryable failure.
///
/// # Arguments
/// * `provider` — The `AnthropicProvider` holding the HTTP client, base URL,
///   `KeyStore`, and retry policy.
/// * `req` — Completion request to send.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `req.messages` contains no non-system
/// message (see `to_anthropic_request`), `DomainError::KeyNotFound` if the API key
/// cannot be resolved via `KeyStore`, `DomainError::Timeout` on a connection/request
/// timeout, `DomainError::RateLimited` on an HTTP 429 response (with `Retry-After`
/// parsed if present), and `DomainError::ProviderError` (with the original error
/// preserved via `#[source]` where available) for any other transport or non-2xx
/// response, or a `200 OK` response whose body does not deserialize into the expected
/// shape.
pub(super) fn complete<'a>(
    provider: &'a AnthropicProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<CompletionResponse, DomainError>> {
    let anthropic_req = to_anthropic_request(req);
    let url = format!("{}/v1/messages", provider.base_url);

    Box::pin(async move {
        let anthropic_req = anthropic_req?;
        let api_key = provider
            .key_store
            .get_key(AnthropicProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("x-api-key", &api_key)
                .header("anthropic-version", ANTHROPIC_VERSION)
                .json(&anthropic_req)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source("request to Anthropic failed", e)
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

        let anthropic_resp: AnthropicResponse = response.json().await.map_err(|e| {
            DomainError::provider_error_with_source("failed to parse Anthropic response", e)
        })?;

        Ok(from_anthropic_response(anthropic_resp))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::ChatMessage;

    #[test]
    fn test_to_anthropic_request_hoists_single_system_message() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
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

        let anthropic_req = to_anthropic_request(&req).expect("request has a non-system message");
        assert_eq!(anthropic_req.model, "claude-opus-4-6");
        assert_eq!(anthropic_req.system, Some("You are helpful.".to_string()));
        assert_eq!(anthropic_req.messages.len(), 1);
        assert_eq!(anthropic_req.messages[0].role, "user");
        assert_eq!(anthropic_req.temperature, Some(0.7));
        assert_eq!(anthropic_req.max_tokens, 1000);
    }

    #[test]
    fn test_to_anthropic_request_joins_multiple_system_messages() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: "Be concise.".to_string().into(),
                },
                ChatMessage {
                    role: Role::System,
                    content: "Be polite.".to_string().into(),
                },
                ChatMessage {
                    role: Role::User,
                    content: "Hello".to_string().into(),
                },
            ],
            temperature: None,
            max_tokens: None,
        };

        let anthropic_req = to_anthropic_request(&req).expect("request has a non-system message");
        assert_eq!(
            anthropic_req.system,
            Some("Be concise.\n\nBe polite.".to_string())
        );
        assert_eq!(anthropic_req.messages.len(), 1);
    }

    /// A request whose messages are entirely `Role::System` would otherwise produce an
    /// empty `messages` array, which Anthropic rejects remotely with an opaque `400
    /// invalid_request_error`. `to_anthropic_request` must reject it locally instead,
    /// with a clear `DomainError::InvalidRequest`.
    #[test]
    fn test_to_anthropic_request_all_system_messages_is_invalid_request() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: "You are helpful.".to_string().into(),
                },
                ChatMessage {
                    role: Role::System,
                    content: "Be concise.".to_string().into(),
                },
            ],
            temperature: None,
            max_tokens: None,
        };

        match to_anthropic_request(&req) {
            Ok(_) => panic!("an all-system request should be rejected locally"),
            Err(e) => assert!(matches!(e, DomainError::InvalidRequest(_))),
        }
    }

    #[test]
    fn test_to_anthropic_request_defaults_max_tokens_when_none() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
            messages: vec![ChatMessage {
                role: Role::User,
                content: "Hello".to_string().into(),
            }],
            temperature: None,
            max_tokens: None,
        };

        let anthropic_req = to_anthropic_request(&req).expect("request has a non-system message");
        assert_eq!(anthropic_req.max_tokens, DEFAULT_MAX_TOKENS);
        assert!(anthropic_req.system.is_none());
    }

    /// `MessageContent::Text` keeps serializing as a bare JSON string, matching
    /// Anthropic's text-only shorthand.
    #[test]
    fn test_to_anthropic_content_text_serializes_as_bare_string() {
        let content = to_anthropic_content(&MessageContent::Text("hello".to_string()));
        assert_eq!(
            serde_json::to_value(&content).unwrap(),
            serde_json::json!("hello")
        );
    }

    /// `MessageContent::Parts` serializes as an array of content blocks: a `text`
    /// block, an `image`/`url` block for `ContentPart::ImageUrl`, and an
    /// `image`/`base64` block for `ContentPart::ImageBase64`.
    #[test]
    fn test_to_anthropic_content_parts_serializes_as_blocks_array() {
        let content = MessageContent::Parts(vec![
            ContentPart::Text("look: ".to_string()),
            ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
            ContentPart::ImageBase64 {
                media_type: "image/png".to_string(),
                data: "abcd".to_string(),
            },
        ]);

        let json = serde_json::to_value(to_anthropic_content(&content)).unwrap();
        assert_eq!(
            json,
            serde_json::json!([
                {"type": "text", "text": "look: "},
                {"type": "image", "source": {"type": "url", "url": "https://example.com/cat.png"}},
                {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "abcd"}},
            ])
        );
    }

    /// End-to-end: a `CompletionRequest` with a mixed text+image message serializes to
    /// the exact Anthropic request body shape.
    #[test]
    fn test_to_anthropic_request_with_image_parts_serializes_full_message_body() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
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

        let anthropic_req = to_anthropic_request(&req).expect("request has a non-system message");
        let json = serde_json::to_value(&anthropic_req).unwrap();
        assert_eq!(
            json["messages"][0]["content"],
            serde_json::json!([
                {"type": "text", "text": "what is this?"},
                {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "abcd"}},
            ])
        );
    }

    /// A system message whose content is `MessageContent::Parts` and contains an image
    /// part has no representable target in Anthropic's flat-string `system` field, so
    /// it must be rejected locally rather than silently dropped.
    #[test]
    fn test_to_anthropic_request_system_message_with_image_part_is_invalid_request() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: MessageContent::Parts(vec![
                        ContentPart::Text("You are helpful.".to_string()),
                        ContentPart::ImageBase64 {
                            media_type: "image/png".to_string(),
                            data: "abcd".to_string(),
                        },
                    ]),
                },
                ChatMessage {
                    role: Role::User,
                    content: "Hello".to_string().into(),
                },
            ],
            temperature: None,
            max_tokens: None,
        };

        match to_anthropic_request(&req) {
            Ok(_) => panic!("a system message containing an image part should be rejected"),
            Err(e) => assert!(matches!(e, DomainError::InvalidRequest(_))),
        }
    }

    /// A plain-text system message (`MessageContent::Text`) is unaffected by the
    /// image-part check and hoists into `system` exactly as before.
    #[test]
    fn test_to_anthropic_request_plain_text_system_message_unchanged() {
        let req = CompletionRequest {
            model: "claude-opus-4-6".to_string(),
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
            temperature: None,
            max_tokens: None,
        };

        let anthropic_req = to_anthropic_request(&req).expect("plain-text system message is valid");
        assert_eq!(anthropic_req.system, Some("You are helpful.".to_string()));
    }

    #[test]
    fn test_role_to_anthropic_str() {
        assert_eq!(role_to_anthropic_str(&Role::User), "user");
        assert_eq!(role_to_anthropic_str(&Role::Assistant), "assistant");
        assert_eq!(
            role_to_anthropic_str(&Role::Tool),
            "user",
            "Role::Tool should fall back to the \"user\" role (documented limitation)"
        );
    }

    #[test]
    fn test_from_anthropic_response_concatenates_text_blocks_and_maps_usage() {
        let resp = AnthropicResponse {
            id: "msg_123".to_string(),
            model: "claude-opus-4-6".to_string(),
            content: vec![
                AnthropicContentBlock {
                    block_type: "text".to_string(),
                    text: "Hi ".to_string(),
                },
                AnthropicContentBlock {
                    block_type: "text".to_string(),
                    text: "there!".to_string(),
                },
            ],
            stop_reason: "end_turn".to_string(),
            usage: AnthropicUsage {
                input_tokens: 20,
                output_tokens: 5,
            },
        };

        let completion = from_anthropic_response(resp);
        assert_eq!(completion.id, "msg_123");
        assert_eq!(completion.model, "claude-opus-4-6");
        assert_eq!(completion.choices.len(), 1);
        assert_eq!(completion.choices[0].index, 0);
        assert_eq!(completion.choices[0].message.role, Role::Assistant);
        assert_eq!(completion.choices[0].message.content.as_text(), "Hi there!");
        assert_eq!(completion.choices[0].finish_reason, "end_turn");
        assert_eq!(completion.usage.prompt_tokens, 20);
        assert_eq!(completion.usage.completion_tokens, 5);
        assert_eq!(completion.usage.total_tokens, 25);
    }
}
