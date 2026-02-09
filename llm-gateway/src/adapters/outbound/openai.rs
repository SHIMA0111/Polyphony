use std::future::Future;
use std::pin::Pin;

use reqwest::Client;
use serde::{Deserialize, Serialize};

use std::sync::Arc;

use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, ModelInfo, Role, Usage,
};
use crate::ports::outbound::key_store::KeyStore;
use crate::ports::outbound::provider::LLMProvider;

/// OpenAI Chat Completions APIアダプター。
///
/// APIキーは `KeyStore` 経由で取得し、ベースURLは環境変数 `OPENAI_BASE_URL` から読み込む。
/// プロバイダー固有の設定はこのアダプター内で完結し、共通Configには含めない。
pub struct OpenAIProvider {
    client: Client,
    base_url: String,
    api_key: String,
}

impl OpenAIProvider {
    /// 新しい `OpenAIProvider` を生成する。
    ///
    /// # Arguments
    /// * `key_store` — APIキー取得に使用するキーストア
    ///
    /// # Environment Variables
    /// * `OPENAI_BASE_URL` — OpenAI APIベースURL（デフォルト: https://api.openai.com）
    pub fn new(key_store: Arc<dyn KeyStore>) -> Result<Self, DomainError> {
        let base_url = std::env::var("OPENAI_BASE_URL")
            .unwrap_or_else(|_| "https://api.openai.com".to_string());
        let api_key = key_store.get_key("openai")?;

        let client = Client::builder()
            // To avoid connection issues like misconfiguration or network failures, we set a timeout of 10 seconds
            .connect_timeout(std::time::Duration::from_secs(10))
            .build()
            .map_err(|e|
                DomainError::ProviderError(format!("failed to create reqwest client: {e}")))?;

        Ok(Self {
            client,
            base_url,
            api_key,
        })
    }
}

// --- OpenAI固有のDTO ---

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

// --- ドメインモデル ↔ OpenAI DTO 変換 ---

fn role_to_string(role: &Role) -> String {
    match role {
        Role::System => "system".to_string(),
        Role::User => "user".to_string(),
        Role::Assistant => "assistant".to_string(),
    }
}

fn string_to_role(s: &str) -> Role {
    match s {
        "system" => Role::System,
        "user" => Role::User,
        "assistant" => Role::Assistant,
        other => {
            tracing::warn!(role = other, "unknown OpenAI role, falling back to User");
            Role::User
        }
    }
}

fn to_openai_request(req: &CompletionRequest) -> OpenAIRequest {
    OpenAIRequest {
        model: req.model.clone(),
        messages: req
            .messages
            .iter()
            .map(|m| OpenAIMessage {
                role: role_to_string(&m.role),
                content: m.content.clone(),
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
                    role: string_to_role(&c.message.role),
                    content: c.message.content,
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

impl LLMProvider for OpenAIProvider {
    fn complete(
        &self,
        req: &CompletionRequest,
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>> {
        let openai_req = to_openai_request(req);
        let url = format!("{}/v1/chat/completions", self.base_url);

        Box::pin(async move {
            let response = self
                .client
                .post(&url)
                .header("Authorization", format!("Bearer {}", self.api_key))
                .json(&openai_req)
                .send()
                .await
                .map_err(|e| {
                    if e.is_timeout() {
                        DomainError::Timeout
                    } else {
                        DomainError::ProviderError(e.to_string())
                    }
                })?;

            if !response.status().is_success() {
                let status = response.status();
                let body = response.text().await.unwrap_or_default();
                let message = serde_json::from_str::<OpenAIErrorResponse>(&body)
                    .map(|e| e.error.message)
                    .unwrap_or(body);
                return Err(DomainError::ProviderError(format!(
                    "OpenAI API error ({status}): {message}"
                )));
            }

            let openai_resp: OpenAIResponse = response.json().await.map_err(|e| {
                DomainError::ProviderError(format!("failed to parse OpenAI response: {e}"))
            })?;

            Ok(from_openai_response(openai_resp))
        })
    }

    fn models(&self) -> Vec<ModelInfo> {
        vec![
            ModelInfo {
                id: "gpt-5.2".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "gpt-5".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "gpt-5-mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "o4-mini".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
            ModelInfo {
                id: "o3".to_string(),
                provider: "openai".to_string(),
                owned_by: "openai".to_string(),
            },
        ]
    }

    fn provider_name(&self) -> &str {
        "openai"
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_to_openai_request() {
        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![
                ChatMessage {
                    role: Role::System,
                    content: "You are helpful.".to_string(),
                },
                ChatMessage {
                    role: Role::User,
                    content: "Hello".to_string(),
                },
            ],
            temperature: Some(0.7),
            max_tokens: Some(1000),
        };

        let openai_req = to_openai_request(&req);
        assert_eq!(openai_req.model, "gpt-5.2");
        assert_eq!(openai_req.messages.len(), 2);
        assert_eq!(openai_req.messages[0].role, "system");
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
        assert_eq!(resp.choices[0].message.content, "Hi there!");
        assert_eq!(resp.usage.prompt_tokens, 20);
        assert_eq!(resp.usage.total_tokens, 25);
    }

    #[test]
    fn test_role_conversions() {
        assert_eq!(role_to_string(&Role::System), "system");
        assert_eq!(role_to_string(&Role::User), "user");
        assert_eq!(role_to_string(&Role::Assistant), "assistant");

        assert_eq!(string_to_role("system"), Role::System);
        assert_eq!(string_to_role("user"), Role::User);
        assert_eq!(string_to_role("assistant"), Role::Assistant);
        assert_eq!(string_to_role("unknown"), Role::User);
    }
}
