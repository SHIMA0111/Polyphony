use serde::Serialize;

use crate::domain::model::{CompletionResponse, ModelInfo, Role};

/// REST API用の補完レスポンスDTO。
#[derive(Serialize)]
pub struct CompletionResponseDto {
    pub id: String,
    pub model: String,
    pub choices: Vec<ChoiceDto>,
    pub usage: UsageDto,
}

/// REST API用の選択肢DTO。
#[derive(Serialize)]
pub struct ChoiceDto {
    pub index: u32,
    pub message: MessageDto,
    pub finish_reason: String,
}

/// REST API用のメッセージDTO。
#[derive(Serialize)]
pub struct MessageDto {
    pub role: String,
    pub content: String,
}

/// REST API用のトークン使用量DTO。
#[derive(Serialize)]
pub struct UsageDto {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// ドメインRoleをREST APIのロール文字列に変換する。
fn role_to_api_string(role: &Role) -> &'static str {
    match role {
        Role::System => "system",
        Role::User => "user",
        Role::Assistant => "assistant",
    }
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
                        role: role_to_api_string(&c.message.role).to_string(),
                        content: c.message.content,
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

/// REST API用のモデル一覧レスポンスDTO。
#[derive(Serialize)]
pub struct ModelsResponseDto {
    pub models: Vec<ModelInfoDto>,
}

/// REST API用のモデル情報DTO。
#[derive(Serialize)]
pub struct ModelInfoDto {
    pub id: String,
    pub provider: String,
    pub owned_by: String,
}

impl From<ModelInfo> for ModelInfoDto {
    fn from(m: ModelInfo) -> Self {
        Self {
            id: m.id,
            provider: m.provider,
            owned_by: m.owned_by,
        }
    }
}
