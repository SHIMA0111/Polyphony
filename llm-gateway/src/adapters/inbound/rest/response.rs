use serde::Serialize;

use crate::domain::model::{CompletionResponse, ModelInfo};

/// Completion response DTO for the REST API.
#[derive(Serialize)]
pub struct CompletionResponseDto {
    pub id: String,
    pub model: String,
    pub choices: Vec<ChoiceDto>,
    pub usage: UsageDto,
}

/// Choice DTO for the REST API.
#[derive(Serialize)]
pub struct ChoiceDto {
    pub index: u32,
    pub message: MessageDto,
    pub finish_reason: String,
}

/// Message DTO for the REST API.
#[derive(Serialize)]
pub struct MessageDto {
    pub role: String,
    pub content: String,
}

/// Token usage DTO for the REST API.
#[derive(Serialize)]
pub struct UsageDto {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

impl From<CompletionResponse> for CompletionResponseDto {
    fn from(resp: CompletionResponse) -> Self {
        Self {
            id: resp.id,
            model: resp.model,
            choices: resp
                .choices
                .into_iter()
                .map(|c| ChoiceDto {
                    index: c.index,
                    message: MessageDto {
                        role: c.message.role.as_str().to_string(),
                        content: c.message.content.as_text(),
                    },
                    finish_reason: c.finish_reason,
                })
                .collect(),
            usage: UsageDto {
                prompt_tokens: resp.usage.prompt_tokens,
                completion_tokens: resp.usage.completion_tokens,
                total_tokens: resp.usage.total_tokens,
            },
        }
    }
}

/// Models list response DTO for the REST API.
#[derive(Serialize)]
pub struct ModelsResponseDto {
    pub models: Vec<ModelInfoDto>,
}

/// Model info DTO for the REST API.
#[derive(Serialize)]
pub struct ModelInfoDto {
    pub id: String,
    pub name: String,
    pub provider: String,
    pub owned_by: String,
}

impl From<ModelInfo> for ModelInfoDto {
    fn from(m: ModelInfo) -> Self {
        Self {
            id: m.id,
            name: m.name,
            provider: m.provider,
            owned_by: m.owned_by,
        }
    }
}
