use serde::Serialize;

use crate::domain::model::{CompletionResponse, ModelInfo, ModelPricing, TokenEstimateResponse};

use super::request::ContentDto;

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
///
/// `content` uses the same `ContentDto` shape as the inbound request DTO (see
/// `adapters::inbound::rest::request::ContentDto`): a completion response's message is
/// almost always plain text today (no provider adapter yet echoes image content back),
/// but reusing the same untagged enum keeps the response contract forward-compatible
/// with a future provider that does, and serializes today's plain-text case as an
/// unchanged bare JSON string.
#[derive(Serialize)]
pub struct MessageDto {
    pub role: String,
    pub content: ContentDto,
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
                        content: c.message.content.into(),
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

/// Model pricing DTO for the REST API. Per-1M-token USD pricing, the project-wide
/// canonical unit — see `domain::model::ModelPricing`.
#[derive(Serialize)]
pub struct ModelPricingDto {
    pub input_price_per_million_tokens: f64,
    pub output_price_per_million_tokens: f64,
    pub currency: String,
}

impl From<ModelPricing> for ModelPricingDto {
    fn from(p: ModelPricing) -> Self {
        Self {
            input_price_per_million_tokens: p.input_price_per_million_tokens,
            output_price_per_million_tokens: p.output_price_per_million_tokens,
            currency: p.currency,
        }
    }
}

/// Model info DTO for the REST API.
#[derive(Serialize)]
pub struct ModelInfoDto {
    pub id: String,
    pub name: String,
    pub provider: String,
    pub owned_by: String,
    /// Maximum context window in tokens, if known. Omitted from the JSON body when
    /// unknown, rather than serialized as `null`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context_window: Option<u32>,
    /// Pricing metadata, if known. Omitted from the JSON body when unknown.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pricing: Option<ModelPricingDto>,
    /// Whether the model accepts image/Vision content parts, if known. Omitted from
    /// the JSON body when unknown; absent must be treated as "no" by consumers.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub supports_image_input: Option<bool>,
}

impl From<ModelInfo> for ModelInfoDto {
    fn from(m: ModelInfo) -> Self {
        Self {
            id: m.id,
            name: m.name,
            provider: m.provider,
            owned_by: m.owned_by,
            context_window: m.context_window,
            pricing: m.pricing.map(ModelPricingDto::from),
            supports_image_input: m.supports_image_input,
        }
    }
}

/// Token estimation response DTO for the REST API.
///
/// `estimated_tokens` is an approximation — see `domain::model::TokenEstimateResponse`
/// and `domain::token_estimator` for the heuristic used to compute it.
#[derive(Serialize)]
pub struct TokenEstimateResponseDto {
    pub model: String,
    pub estimated_tokens: u32,
}

impl From<TokenEstimateResponse> for TokenEstimateResponseDto {
    fn from(resp: TokenEstimateResponse) -> Self {
        Self {
            model: resp.model,
            estimated_tokens: resp.estimated_tokens,
        }
    }
}
