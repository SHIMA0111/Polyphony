use serde::Deserialize;

use crate::domain::model::{ChatMessage, CompletionRequest};

/// REST API用の補完リクエストDTO。
#[derive(Debug, Deserialize)]
pub struct CompletionRequestDto {
    pub model: String,
    pub messages: Vec<ChatMessage>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

impl From<CompletionRequestDto> for CompletionRequest {
    fn from(dto: CompletionRequestDto) -> Self {
        Self {
            model: dto.model,
            messages: dto.messages,
            temperature: dto.temperature,
            max_tokens: dto.max_tokens,
        }
    }
}
