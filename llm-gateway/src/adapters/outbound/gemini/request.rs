use futures::future::BoxFuture;
use serde::{Deserialize, Serialize};

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, ContentPart, MessageContent, Role,
    Usage,
};

use super::GeminiProvider;

// --- Gemini-specific DTOs ---

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct GeminiRequest {
    contents: Vec<GeminiRequestContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    system_instruction: Option<GeminiRequestContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    generation_config: Option<GeminiGenerationConfig>,
}

/// A single outbound `Content` object in Gemini's request shape.
///
/// `role` is `None`/omitted when this represents the top-level `systemInstruction`
/// (Gemini's `systemInstruction` object has no `role` field), and
/// `Some("user"|"model"|"function")` for entries in `contents`.
///
/// This is the *outbound* counterpart of [`GeminiContent`] (used for parsing
/// responses): the two are kept separate because `parts` carries a richer,
/// Vision-capable shape ([`GeminiPartDto`]) on the outbound side than the
/// text-only shape Gemini ever echoes back in a response.
#[derive(Serialize)]
struct GeminiRequestContent {
    #[serde(skip_serializing_if = "Option::is_none")]
    role: Option<String>,
    parts: Vec<GeminiPartDto>,
}

/// A single outbound `Part` object in Gemini's `generateContent` request shape.
///
/// Untagged: Gemini distinguishes part kinds by which key is present (`text`,
/// `inlineData`, or `fileData`), not by an explicit discriminator field, so
/// `#[serde(untagged)]` (which serializes only the active variant's fields, without a
/// wrapping tag) matches the wire format exactly.
#[derive(Serialize)]
#[serde(untagged)]
enum GeminiPartDto {
    Text {
        text: String,
    },
    InlineData {
        #[serde(rename = "inlineData")]
        inline_data: GeminiInlineData,
    },
    FileData {
        #[serde(rename = "fileData")]
        file_data: GeminiFileData,
    },
}

/// Gemini's `inlineData` part: a base64-encoded image (or other blob) embedded
/// directly in the request.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct GeminiInlineData {
    mime_type: String,
    data: String,
}

/// Gemini's `fileData` part: a reference to an image (or other file) by URI.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct GeminiFileData {
    file_uri: String,
}

/// A single `Content` object in Gemini's response shape (`candidates[].content`).
///
/// See [`GeminiRequestContent`] for the outbound counterpart used when building a
/// request; this type only needs to deserialize the text-only shape Gemini responses
/// ever contain.
#[derive(Deserialize, Clone)]
struct GeminiContent {
    #[serde(default)]
    role: Option<String>,
    parts: Vec<GeminiPart>,
}

#[derive(Deserialize, Clone)]
struct GeminiPart {
    text: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct GeminiGenerationConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_output_tokens: Option<u32>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiResponse {
    /// Absent entirely (not just empty) on some safety-blocked responses, hence the
    /// `#[serde(default)]`.
    #[serde(default)]
    candidates: Vec<GeminiCandidate>,
    usage_metadata: Option<GeminiUsageMetadata>,
    response_id: Option<String>,
    prompt_feedback: Option<GeminiPromptFeedback>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiCandidate {
    content: Option<GeminiContent>,
    finish_reason: Option<String>,
    index: Option<u32>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiUsageMetadata {
    prompt_token_count: u32,
    candidates_token_count: u32,
    total_token_count: u32,
}

/// Carries Gemini's safety/policy block reason (e.g. `"SAFETY"`), present on the
/// top-level `promptFeedback` field when a request is blocked before any candidate is
/// produced (`candidates` is then empty or absent).
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GeminiPromptFeedback {
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

// --- Domain model <-> Gemini DTO conversion ---

/// Converts a domain `Role` to a Gemini `Content.role` string.
///
/// This is a temporary text-only simplification: full Gemini function-calling
/// (`functionCall`/`functionResponse` parts) is out of scope until a future
/// tool-calling phase. Gemini's documented `Content.role` values are only `"user"` and
/// `"model"` (no `"function"` role for request content), so `Role::Tool` is sent as
/// plain text under Gemini's `"user"` role -- mirroring the Anthropic adapter's
/// documented best-effort fallback (`role_to_anthropic_str`) rather than risking a live
/// 400 from an undocumented role string.
///
/// # Arguments
/// * `role` — Domain role to convert. Must not be `Role::System` — system messages are
///   always extracted into `systemInstruction` by `to_gemini_request` before this
///   function is ever called for the remaining messages.
///
/// # Returns
/// - `User` → `"user"`
/// - `Assistant` → `"model"`
/// - `Tool` → `"user"` (documented best-effort fallback: see above)
fn role_to_gemini_role(role: &Role) -> &'static str {
    match role {
        Role::User => "user",
        Role::Assistant => "model",
        Role::Tool => "user",
        Role::System => {
            debug_assert!(
                false,
                "Role::System must be extracted into system_instruction before \
                 role_to_gemini_role is called"
            );
            "user"
        }
    }
}

/// Converts a Gemini `Content.role` string back to a domain `Role`.
///
/// # Arguments
/// * `s` — Role string as received from Gemini (e.g. `"model"`, `"user"`).
///
/// # Returns
/// `"model"` → `Assistant`, `"user"` → `User`, `"function"` → `Tool`; any other string
/// falls back to `User` with a `tracing::warn!`, mirroring the OpenAI adapter's lenient
/// fallback behavior for unknown role strings.
fn gemini_role_to_role(s: &str) -> Role {
    match s {
        "model" => Role::Assistant,
        "user" => Role::User,
        "function" => Role::Tool,
        other => {
            tracing::warn!(role = other, "unknown Gemini role, falling back to User");
            Role::User
        }
    }
}

/// Converts a domain `MessageContent` into Gemini's `parts` array.
///
/// `MessageContent::Text` becomes a single-element `parts` array (Gemini has no bare
/// string shorthand for `content`, unlike OpenAI/Anthropic — every message is always a
/// `parts` array). `MessageContent::Parts` becomes one `parts` entry per `ContentPart`
/// (see `to_gemini_part`).
///
/// # Errors
/// Never fails: every `ContentPart` variant has a representable Gemini part (see
/// `to_gemini_part`).
fn to_gemini_parts(content: &MessageContent) -> Vec<GeminiPartDto> {
    match content {
        MessageContent::Text(s) => vec![GeminiPartDto::Text { text: s.clone() }],
        MessageContent::Parts(parts) => parts.iter().map(to_gemini_part).collect(),
    }
}

/// Converts a single domain `ContentPart` into a Gemini part.
///
/// `ContentPart::ImageBase64` maps to an `inlineData` part (Gemini's native
/// inline-image shape); `ContentPart::ImageUrl` maps to a `fileData` part (Gemini's
/// reference-by-URI shape, intended for Gemini File API URIs, but structurally the
/// only Gemini part type that carries a bare URL string).
fn to_gemini_part(part: &ContentPart) -> GeminiPartDto {
    match part {
        ContentPart::Text(text) => GeminiPartDto::Text { text: text.clone() },
        ContentPart::ImageUrl(url) => GeminiPartDto::FileData {
            file_data: GeminiFileData {
                file_uri: url.clone(),
            },
        },
        ContentPart::ImageBase64 { media_type, data } => GeminiPartDto::InlineData {
            inline_data: GeminiInlineData {
                mime_type: media_type.clone(),
                data: data.clone(),
            },
        },
    }
}

/// Converts a provider-agnostic `CompletionRequest` into a Gemini `generateContent`
/// request body.
///
/// All `Role::System` messages are joined (with `"\n"`) into a single top-level
/// `system_instruction`, since Gemini does not accept a `"system"` role inside
/// `contents`; all other messages become `contents` entries in original order.
///
/// # Arguments
/// * `req` — Provider-agnostic completion request to convert.
///
/// # Returns
/// The equivalent `GeminiRequest` body, ready to be serialized and sent to
/// `generateContent`.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `req.messages` contains no
/// non-`Role::System` message: Gemini's `generateContent` requires a non-empty
/// `contents` array, and a request built from only system messages would
/// otherwise be sent with `contents: []`, surfacing as an opaque remote HTTP
/// 400 instead of a clear domain error. Also returns `DomainError::InvalidRequest`
/// if a system message's content is `MessageContent::Parts` containing any
/// non-`ContentPart::Text` part (see `system_message_text`), since silently
/// dropping an image from a system message would misrepresent what was sent.
pub(super) fn to_gemini_request(req: &CompletionRequest) -> Result<GeminiRequest, DomainError> {
    let mut system_texts = Vec::new();
    let mut contents = Vec::new();

    for m in &req.messages {
        match m.role {
            Role::System => system_texts.push(super::super::system_message_text(&m.content)?),
            _ => contents.push(GeminiRequestContent {
                role: Some(role_to_gemini_role(&m.role).to_string()),
                parts: to_gemini_parts(&m.content),
            }),
        }
    }

    if contents.is_empty() {
        return Err(DomainError::InvalidRequest(
            "messages must contain at least one non-system message".to_string(),
        ));
    }

    let system_instruction = if system_texts.is_empty() {
        None
    } else {
        Some(GeminiRequestContent {
            role: None,
            parts: vec![GeminiPartDto::Text {
                text: system_texts.join("\n"),
            }],
        })
    };

    let generation_config = if req.temperature.is_none() && req.max_tokens.is_none() {
        None
    } else {
        Some(GeminiGenerationConfig {
            temperature: req.temperature,
            max_output_tokens: req.max_tokens,
        })
    };

    Ok(GeminiRequest {
        contents,
        system_instruction,
        generation_config,
    })
}

/// Converts a Gemini `generateContent` response into a provider-agnostic
/// `CompletionResponse`.
///
/// # Arguments
/// * `resp` — Parsed Gemini response body.
/// * `model` — Model ID that was requested (Gemini's response does not echo it back).
///
/// # Errors
/// Returns `DomainError::ProviderError` when `resp.candidates` is empty: if
/// `resp.prompt_feedback`'s `block_reason` is present, the error message includes it
/// (a safety/policy block); otherwise the message reports an unexpected empty
/// response.
fn from_gemini_response(
    resp: GeminiResponse,
    model: &str,
) -> Result<CompletionResponse, DomainError> {
    let Some(candidate) = resp.candidates.into_iter().next() else {
        let message = match resp.prompt_feedback.and_then(|f| f.block_reason) {
            Some(reason) => format!("Gemini blocked the request (reason: {reason})"),
            None => "Gemini returned no candidates in response".to_string(),
        };
        return Err(DomainError::provider_error(message));
    };

    // Read the candidate's role before `content` is moved out by the `.map` below.
    let role = gemini_role_to_role(
        candidate
            .content
            .as_ref()
            .and_then(|c| c.role.as_deref())
            .unwrap_or("model"),
    );

    let text = candidate
        .content
        .map(|c| {
            c.parts
                .into_iter()
                .map(|p| p.text)
                .collect::<Vec<_>>()
                .join("")
        })
        .unwrap_or_default();

    let finish_reason = candidate
        .finish_reason
        .unwrap_or_else(|| "stop".to_string());

    let usage = resp
        .usage_metadata
        .map(|u| Usage {
            prompt_tokens: u.prompt_token_count,
            completion_tokens: u.candidates_token_count,
            total_tokens: u.total_token_count,
        })
        .unwrap_or_else(|| {
            tracing::warn!("Gemini response is missing usageMetadata, defaulting to zero usage");
            Usage {
                prompt_tokens: 0,
                completion_tokens: 0,
                total_tokens: 0,
            }
        });

    // Gemini's `generateContent` response does not always include a stable response
    // identifier (`responseId`); fall back to a freshly generated UUID so callers can
    // still rely on `CompletionResponse::id` being present and unique.
    let id = resp
        .response_id
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    Ok(CompletionResponse {
        id,
        model: model.to_string(),
        choices: vec![Choice {
            index: candidate.index.unwrap_or(0),
            message: ChatMessage {
                role,
                content: text.into(),
            },
            finish_reason,
        }],
        usage,
    })
}

/// Executes a chat completion request against Google's Generative Language API
/// (`generateContent`).
///
/// The outbound request is retried with bounded exponential backoff on `429`/`5xx`
/// responses via `send_with_retry`; a `429` that persists after retries are exhausted
/// is mapped to `DomainError::RateLimited` and a `5xx` to `DomainError::ProviderError`,
/// same as a non-retryable failure.
///
/// # Arguments
/// * `provider` — The `GeminiProvider` holding the HTTP client, base URL, `KeyStore`,
///   and retry policy.
/// * `req` — Completion request to send.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `req.messages` contains no non-system
/// message (see `to_gemini_request`), `DomainError::KeyNotFound` if the API key
/// cannot be resolved via `KeyStore`, `DomainError::Timeout` on a
/// connection/request timeout, `DomainError::RateLimited` on an HTTP 429 response
/// (with `Retry-After` parsed if present), and `DomainError::ProviderError` (with
/// the original error preserved via `#[source]` where available) for any other
/// transport or non-2xx response, or an empty/safety-blocked `candidates` list.
pub(super) fn complete<'a>(
    provider: &'a GeminiProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<CompletionResponse, DomainError>> {
    let model = req.model.clone();
    let url = format!(
        "{}/v1beta/models/{model}:generateContent",
        provider.base_url
    );
    let gemini_req_result = to_gemini_request(req);

    Box::pin(async move {
        let gemini_req = gemini_req_result?;
        let api_key = provider.key_store.get_key(GeminiProvider::PROVIDER_NAME)?;

        let send_request = || {
            provider
                .client
                .post(&url)
                .header("x-goog-api-key", api_key.as_str())
                .json(&gemini_req)
                .send()
        };

        let response = send_with_retry(&provider.retry_policy, send_request)
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    DomainError::Timeout
                } else {
                    DomainError::provider_error_with_source("request to Gemini failed", e)
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

        let gemini_resp: GeminiResponse = response.json().await.map_err(|e| {
            DomainError::provider_error_with_source("failed to parse Gemini response", e)
        })?;

        from_gemini_response(gemini_resp, &model)
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::ChatMessage;

    #[test]
    fn test_to_gemini_request_extracts_and_joins_system_messages() {
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: "You are helpful.".to_string().into(),
                },
                ChatMessage {
                    role: Role::System,
                    content: "Be concise.".to_string().into(),
                },
                ChatMessage {
                    role: Role::User,
                    content: "Hello".to_string().into(),
                },
                ChatMessage {
                    role: Role::Assistant,
                    content: "Hi!".to_string().into(),
                },
                ChatMessage {
                    role: Role::User,
                    content: "How are you?".to_string().into(),
                },
            ],
            temperature: Some(0.5),
            max_tokens: Some(256),
        };

        let gemini_req = to_gemini_request(&req).expect("request has non-system messages");

        let system_instruction = gemini_req
            .system_instruction
            .expect("system messages should produce a system_instruction");
        assert!(system_instruction.role.is_none());
        assert_eq!(system_instruction.parts.len(), 1);
        assert_eq!(
            serde_json::to_value(&system_instruction.parts[0]).unwrap(),
            serde_json::json!({"text": "You are helpful.\nBe concise."})
        );

        assert_eq!(gemini_req.contents.len(), 3);
        let roles: Vec<Option<String>> =
            gemini_req.contents.iter().map(|c| c.role.clone()).collect();
        assert_eq!(
            roles,
            vec![
                Some("user".to_string()),
                Some("model".to_string()),
                Some("user".to_string())
            ]
        );

        let generation_config = gemini_req
            .generation_config
            .expect("temperature/max_tokens should produce a generation_config");
        assert_eq!(generation_config.temperature, Some(0.5));
        assert_eq!(generation_config.max_output_tokens, Some(256));
    }

    #[test]
    fn test_to_gemini_request_no_system_messages_omits_system_instruction() {
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
            messages: vec![ChatMessage {
                role: Role::User,
                content: "Hello".to_string().into(),
            }],
            temperature: None,
            max_tokens: None,
        };

        let gemini_req = to_gemini_request(&req).expect("request has a non-system message");
        assert!(gemini_req.system_instruction.is_none());
        assert!(gemini_req.generation_config.is_none());
    }

    /// `MessageContent::Text` becomes a single-element `parts` array with a `text`
    /// entry (Gemini has no bare-string shorthand, unlike OpenAI/Anthropic).
    #[test]
    fn test_to_gemini_parts_text_becomes_single_text_part() {
        let parts = to_gemini_parts(&MessageContent::Text("hello".to_string()));
        let json = serde_json::to_value(&parts).unwrap();
        assert_eq!(json, serde_json::json!([{"text": "hello"}]));
    }

    /// `MessageContent::Parts` maps each `ContentPart` to its Gemini counterpart:
    /// `ImageBase64` becomes `inlineData`, `ImageUrl` becomes `fileData`.
    #[test]
    fn test_to_gemini_parts_maps_image_parts() {
        let content = MessageContent::Parts(vec![
            ContentPart::Text("look: ".to_string()),
            ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
            ContentPart::ImageBase64 {
                media_type: "image/png".to_string(),
                data: "abcd".to_string(),
            },
        ]);

        let json = serde_json::to_value(to_gemini_parts(&content)).unwrap();
        assert_eq!(
            json,
            serde_json::json!([
                {"text": "look: "},
                {"fileData": {"fileUri": "https://example.com/cat.png"}},
                {"inlineData": {"mimeType": "image/png", "data": "abcd"}},
            ])
        );
    }

    /// End-to-end: a `CompletionRequest` with a mixed text+image message serializes to
    /// the exact Gemini request body shape.
    #[test]
    fn test_to_gemini_request_with_image_parts_serializes_full_message_body() {
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
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

        let gemini_req = to_gemini_request(&req).expect("request has a non-system message");
        let json = serde_json::to_value(&gemini_req).unwrap();
        assert_eq!(
            json["contents"][0]["parts"],
            serde_json::json!([
                {"text": "what is this?"},
                {"inlineData": {"mimeType": "image/png", "data": "abcd"}},
            ])
        );
    }

    #[test]
    fn test_to_gemini_request_system_only_is_invalid_request() {
        // A request built from only Role::System messages would otherwise produce
        // an empty `contents` array, which Gemini's generateContent API rejects
        // with an opaque remote HTTP 400. to_gemini_request must reject this
        // locally with a clear DomainError::InvalidRequest instead.
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
            messages: vec![ChatMessage {
                role: Role::System,
                content: "You are helpful.".to_string().into(),
            }],
            temperature: None,
            max_tokens: None,
        };

        let err = match to_gemini_request(&req) {
            Ok(_) => panic!("a system-only request should be rejected as invalid"),
            Err(e) => e,
        };

        match err {
            DomainError::InvalidRequest(message) => {
                assert!(
                    message.contains("non-system"),
                    "expected the error to mention the missing non-system message, got: {message}"
                );
            }
            other => panic!("expected DomainError::InvalidRequest, got {other:?}"),
        }
    }

    /// A system message whose content is `MessageContent::Parts` containing an image
    /// part must be rejected: silently dropping the image (as `as_text()` would) would
    /// misrepresent what was actually sent to Gemini.
    #[test]
    fn test_to_gemini_request_system_message_with_image_part_is_invalid_request() {
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
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

        let err = match to_gemini_request(&req) {
            Ok(_) => panic!("a system message containing an image part should be rejected"),
            Err(e) => e,
        };

        assert!(matches!(err, DomainError::InvalidRequest(_)));
    }

    /// A plain-text (non-`Parts`) system message is unaffected by the image-part
    /// rejection and still hoists into `system_instruction` as before.
    #[test]
    fn test_to_gemini_request_plain_text_system_message_unchanged() {
        let req = CompletionRequest {
            model: "gemini-3-pro".to_string(),
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

        let gemini_req = to_gemini_request(&req).expect("plain text system message is valid");
        let system_instruction = gemini_req
            .system_instruction
            .expect("system message should produce a system_instruction");
        assert_eq!(
            serde_json::to_value(&system_instruction.parts[0]).unwrap(),
            serde_json::json!({"text": "You are helpful."})
        );
    }

    #[test]
    fn test_role_to_gemini_role_and_back_round_trip() {
        assert_eq!(role_to_gemini_role(&Role::User), "user");
        assert_eq!(role_to_gemini_role(&Role::Assistant), "model");

        assert_eq!(gemini_role_to_role("user"), Role::User);
        assert_eq!(gemini_role_to_role("model"), Role::Assistant);
    }

    #[test]
    fn test_role_to_gemini_role_tool_falls_back_to_user() {
        // Gemini's documented Content.role values are only "user"/"model" (no
        // "function" role for request content), so Role::Tool must map to "user"
        // rather than an undocumented role string that risks a live 400.
        assert_eq!(role_to_gemini_role(&Role::Tool), "user");
    }

    #[test]
    fn test_gemini_role_to_role_unknown_falls_back_to_user() {
        assert_eq!(gemini_role_to_role("totally-unknown-role"), Role::User);
    }

    #[test]
    fn test_from_gemini_response_maps_success_fixture() {
        let resp = GeminiResponse {
            candidates: vec![GeminiCandidate {
                content: Some(GeminiContent {
                    role: Some("model".to_string()),
                    parts: vec![GeminiPart {
                        text: "Hi there!".to_string(),
                    }],
                }),
                finish_reason: Some("STOP".to_string()),
                index: Some(0),
            }],
            usage_metadata: Some(GeminiUsageMetadata {
                prompt_token_count: 10,
                candidates_token_count: 4,
                total_token_count: 14,
            }),
            response_id: Some("resp-123".to_string()),
            prompt_feedback: None,
        };

        let completion = from_gemini_response(resp, "gemini-3-pro")
            .expect("a response with a candidate should map successfully");

        assert_eq!(completion.id, "resp-123");
        assert_eq!(completion.model, "gemini-3-pro");
        assert_eq!(completion.choices.len(), 1);
        assert_eq!(completion.choices[0].message.role, Role::Assistant);
        assert_eq!(completion.choices[0].message.content.as_text(), "Hi there!");
        assert_eq!(completion.choices[0].finish_reason, "STOP");
        assert_eq!(completion.usage.prompt_tokens, 10);
        assert_eq!(completion.usage.completion_tokens, 4);
        assert_eq!(completion.usage.total_tokens, 14);
    }

    #[test]
    fn test_from_gemini_response_missing_response_id_generates_uuid() {
        let resp = GeminiResponse {
            candidates: vec![GeminiCandidate {
                content: Some(GeminiContent {
                    role: Some("model".to_string()),
                    parts: vec![GeminiPart {
                        text: "Hi!".to_string(),
                    }],
                }),
                finish_reason: None,
                index: None,
            }],
            usage_metadata: None,
            response_id: None,
            prompt_feedback: None,
        };

        let completion = from_gemini_response(resp, "gemini-3-flash")
            .expect("a response with a candidate should map successfully");

        assert!(!completion.id.is_empty());
        assert_eq!(completion.choices[0].finish_reason, "stop");
        assert_eq!(completion.usage.total_tokens, 0);
    }

    #[test]
    fn test_from_gemini_response_empty_candidates_with_block_reason_is_provider_error() {
        let resp = GeminiResponse {
            candidates: vec![],
            usage_metadata: None,
            response_id: None,
            prompt_feedback: Some(GeminiPromptFeedback {
                block_reason: Some("SAFETY".to_string()),
            }),
        };

        let err = from_gemini_response(resp, "gemini-3-pro")
            .expect_err("empty candidates should be an error, not a panic");

        match err {
            DomainError::ProviderError { message, .. } => {
                assert!(
                    message.contains("SAFETY"),
                    "expected the block reason to be surfaced, got: {message}"
                );
            }
            other => panic!("expected DomainError::ProviderError, got {other:?}"),
        }
    }

    #[test]
    fn test_from_gemini_response_empty_candidates_without_block_reason_is_provider_error() {
        let resp = GeminiResponse {
            candidates: vec![],
            usage_metadata: None,
            response_id: None,
            prompt_feedback: None,
        };

        let err = from_gemini_response(resp, "gemini-3-pro")
            .expect_err("empty candidates should be an error, not a panic");

        assert!(matches!(err, DomainError::ProviderError { .. }));
    }
}
