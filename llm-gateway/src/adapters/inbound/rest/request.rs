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
                    role: Role::parse(&m.role)?,
                    content: m.content.into(),
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

#[cfg(test)]
mod tests {
    use super::*;

    /// Regression test: the REST-inbound parser previously rejected the `"developer"`
    /// role while the OpenAI adapter's parser accepted it. Both now share `Role::parse`,
    /// so `"developer"` is accepted here too.
    #[test]
    fn test_into_domain_accepts_developer_role() {
        let dto = CompletionRequestDto {
            model: "gpt-5.2".to_string(),
            messages: vec![MessageDto {
                role: "developer".to_string(),
                content: "be concise".to_string(),
            }],
            temperature: None,
            max_tokens: None,
        };

        let req = dto
            .into_domain()
            .expect("developer role should be accepted");
        assert_eq!(req.messages[0].role, Role::System);
    }
}
