use serde::Deserialize;

use crate::domain::error::DomainError;
use crate::domain::model::{ChatMessage, CompletionRequest, Role};

/// Completion request DTO for the REST API.
#[derive(Debug, Deserialize)]
pub struct CompletionRequestDto {
    pub model: String,
    pub messages: Vec<MessageDto>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

/// Message DTO for the REST API.
///
/// Roles are received as strings and converted to the domain `Role`.
/// The REST API accepts OpenAI-compatible strings ("system", "user", "assistant", "tool").
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
        "tool" => Ok(Role::Tool),
        other => Err(DomainError::InvalidRequest(format!(
            "unknown role: {other}"
        ))),
    }
}

impl CompletionRequestDto {
    /// Converts this DTO into a domain model.
    ///
    /// # Errors
    /// Returns `DomainError::InvalidRequest` if an unknown role string is encountered.
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
