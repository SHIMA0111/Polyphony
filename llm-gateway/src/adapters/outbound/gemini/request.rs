use futures::future::BoxFuture;
use serde::{Deserialize, Serialize};

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, Role, Usage,
};

use super::GeminiProvider;

// --- Gemini-specific DTOs ---

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct GeminiRequest {
    contents: Vec<GeminiContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    system_instruction: Option<GeminiContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    generation_config: Option<GeminiGenerationConfig>,
}

/// A single `Content` object in Gemini's request/response shape.
///
/// `role` is `None`/omitted when this `GeminiContent` represents the top-level
/// `systemInstruction` (Gemini's `systemInstruction` object has no `role` field), and
/// `Some("user"|"model"|"function")` for entries in `contents`.
#[derive(Serialize, Deserialize, Clone)]
struct GeminiContent {
    #[serde(skip_serializing_if = "Option::is_none")]
    role: Option<String>,
    parts: Vec<GeminiPart>,
}

#[derive(Serialize, Deserialize, Clone)]
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

/// Converts a provider-agnostic `CompletionRequest` into a Gemini `generateContent`
/// request body.
///
/// All `Role::System` messages are joined (with `"\n"`) into a single top-level
/// `system_instruction`, since Gemini does not accept a `"system"` role inside
/// `contents`; all other messages become `contents` entries in original order.
fn to_gemini_request(req: &CompletionRequest) -> GeminiRequest {
    let mut system_texts = Vec::new();
    let mut contents = Vec::new();

    for m in &req.messages {
        match m.role {
            Role::System => system_texts.push(m.content.as_text()),
            _ => contents.push(GeminiContent {
                role: Some(role_to_gemini_role(&m.role).to_string()),
                parts: vec![GeminiPart {
                    text: m.content.as_text(),
                }],
            }),
        }
    }

    let system_instruction = if system_texts.is_empty() {
        None
    } else {
        Some(GeminiContent {
            role: None,
            parts: vec![GeminiPart {
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

    GeminiRequest {
        contents,
        system_instruction,
        generation_config,
    }
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
/// Returns `DomainError::KeyNotFound` if the API key cannot be resolved via
/// `KeyStore`, `DomainError::Timeout` on a connection/request timeout,
/// `DomainError::RateLimited` on an HTTP 429 response (with `Retry-After` parsed if
/// present), and `DomainError::ProviderError` (with the original error preserved via
/// `#[source]` where available) for any other transport or non-2xx response, or an
/// empty/safety-blocked `candidates` list.
pub(super) fn complete<'a>(
    provider: &'a GeminiProvider,
    req: &CompletionRequest,
) -> BoxFuture<'a, Result<CompletionResponse, DomainError>> {
    let gemini_req = to_gemini_request(req);
    let model = req.model.clone();
    let url = format!(
        "{}/v1beta/models/{model}:generateContent",
        provider.base_url
    );

    Box::pin(async move {
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

        let gemini_req = to_gemini_request(&req);

        let system_instruction = gemini_req
            .system_instruction
            .expect("system messages should produce a system_instruction");
        assert!(system_instruction.role.is_none());
        assert_eq!(system_instruction.parts.len(), 1);
        assert_eq!(
            system_instruction.parts[0].text,
            "You are helpful.\nBe concise."
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

        let gemini_req = to_gemini_request(&req);
        assert!(gemini_req.system_instruction.is_none());
        assert!(gemini_req.generation_config.is_none());
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
