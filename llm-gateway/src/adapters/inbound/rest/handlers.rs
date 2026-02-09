use std::sync::Arc;

use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;

use crate::domain::error::DomainError;
use crate::ports::inbound::completion::CompletionUseCase;

use super::request::CompletionRequestDto;
use super::response::{CompletionResponseDto, ModelsResponseDto, ModelInfoDto};

/// Shared application state.
pub type AppState = Arc<dyn CompletionUseCase>;

/// Health check endpoint.
///
/// `GET /health` — Returns service liveness status.
pub async fn health() -> impl IntoResponse {
    Json(serde_json::json!({"status": "ok"}))
}

/// List models endpoint.
///
/// `GET /models` — Returns available models from all providers.
pub async fn list_models(State(service): State<AppState>) -> impl IntoResponse {
    let models = service
        .list_models()
        .into_iter()
        .map(ModelInfoDto::from)
        .collect();
    Json(ModelsResponseDto { models })
}

/// Chat completion endpoint.
///
/// `POST /completions` — Sends a chat completion request to an LLM provider.
pub async fn complete(
    State(service): State<AppState>,
    Json(dto): Json<CompletionRequestDto>,
) -> Result<impl IntoResponse, AppError> {
    let req = dto.into_domain()?;
    let resp = service.complete(req).await?;
    Ok(Json(CompletionResponseDto::from(resp)))
}

/// Wrapper that converts domain errors into HTTP responses.
pub struct AppError(DomainError);

impl From<DomainError> for AppError {
    fn from(err: DomainError) -> Self {
        Self(err)
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        let (status, message) = match &self.0 {
            DomainError::InvalidRequest(msg) => (StatusCode::BAD_REQUEST, msg.clone()),
            DomainError::ModelNotFound(model) => {
                (StatusCode::NOT_FOUND, format!("model not found: {model}"))
            }
            DomainError::KeyNotFound(provider) => (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("API key not configured for {provider}"),
            ),
            DomainError::Timeout => {
                (StatusCode::GATEWAY_TIMEOUT, "request timed out".to_string())
            }
            DomainError::ProviderError(msg) => {
                (StatusCode::BAD_GATEWAY, msg.clone())
            }
        };

        tracing::error!(error = %self.0, "request failed");

        (status, Json(serde_json::json!({"error": message}))).into_response()
    }
}
