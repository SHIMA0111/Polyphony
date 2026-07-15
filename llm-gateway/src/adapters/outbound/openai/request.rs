use futures::future::BoxFuture;
use serde::{Deserialize, Serialize};

use crate::adapters::outbound::http_retry::send_with_retry;
use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, Role, Usage,
};

use super::OpenAIProvider;

// --- OpenAI-specific DTOs ---

#[derive(Serialize)]
struct OpenAIRequest {
    model: String,
    messages: Vec<OpenAIMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_tokens: Option<u32>,
}

#[derive(Serialize, Deserialize)]
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

fn to_openai_request(req: &CompletionRequest) -> OpenAIRequest {
    OpenAIRequest {
        model: req.model.clone(),
        messages: req
            .messages
            .iter()
            .map(|m| OpenAIMessage {
                role: role_to_openai_str(&m.role).to_string(),
                content: m.content.as_text(),
            })
            .collect(),
        temperature: req.temperature,
        max_tokens: req.max_tokens,
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
