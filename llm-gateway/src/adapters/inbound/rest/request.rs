use serde::Deserialize;

use crate::domain::error::DomainError;
use crate::domain::model::{ChatMessage, CompletionRequest, Role};

/// REST API用の補完リクエストDTO。
#[derive(Debug, Deserialize)]
pub struct CompletionRequestDto {
    pub model: String,
    pub messages: Vec<MessageDto>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

/// REST API用のメッセージDTO。
///
/// ロールは文字列で受け取り、ドメインの `Role` に変換する。
/// REST APIはOpenAI互換の文字列（"system", "user", "assistant"）を受け付ける。
#[derive(Debug, Deserialize)]
pub struct MessageDto {
    pub role: String,
    pub content: String,
}

fn parse_role(s: &str) -> Result<Role, DomainError> {
    match s {
        "system" => Ok(Role::System),
        "user" => Ok(Role::User),
        "assistant" => Ok(Role::Assistant),
        other => Err(DomainError::InvalidRequest(format!(
            "unknown role: {other}"
        ))),
    }
}

impl CompletionRequestDto {
    /// DTOからドメインモデルに変換する。
    ///
    /// # Errors
    /// 不明なロール文字列が含まれる場合 `DomainError::InvalidRequest` を返す。
    pub fn into_domain(self) -> Result<CompletionRequest, DomainError> {
        let messages = self
            .messages
            .into_iter()
            .map(|m| {
                Ok(ChatMessage {
                    role: parse_role(&m.role)?,
                    content: m.content,
                })
            })
            .collect::<Result<Vec<_>, DomainError>>()?;

        Ok(CompletionRequest {
            model: self.model,
            messages,
            temperature: self.temperature,
            max_tokens: self.max_tokens,
        })
    }
}
