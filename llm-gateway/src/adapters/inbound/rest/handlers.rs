use std::sync::Arc;

use axum::Json;
use axum::extract::State;
use axum::http::{HeaderValue, StatusCode};
use axum::response::IntoResponse;

use crate::domain::error::DomainError;
use crate::ports::inbound::completion::CompletionUseCase;

use super::request::CompletionRequestDto;
use super::response::{CompletionResponseDto, ModelInfoDto, ModelsResponseDto};

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
        .await
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
        let status = match &self.0 {
            DomainError::InvalidRequest(_) => StatusCode::BAD_REQUEST,
            DomainError::ModelNotFound(_) => StatusCode::NOT_FOUND,
            DomainError::KeyNotFound(_) => StatusCode::INTERNAL_SERVER_ERROR,
            DomainError::Timeout => StatusCode::GATEWAY_TIMEOUT,
            DomainError::ProviderError { .. } => StatusCode::BAD_GATEWAY,
            DomainError::RateLimited { .. } => StatusCode::TOO_MANY_REQUESTS,
        };
        let retry_after_secs = match &self.0 {
            DomainError::RateLimited { retry_after_secs } => *retry_after_secs,
            _ => None,
        };
        let message = self.0.to_string();

        tracing::error!(error = %self.0, "request failed");

        let mut response = (status, Json(serde_json::json!({"error": message}))).into_response();

        if let Some(secs) = retry_after_secs
            && let Ok(value) = HeaderValue::from_str(&secs.to_string())
        {
            response
                .headers_mut()
                .insert(axum::http::header::RETRY_AFTER, value);
        }

        response
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rate_limited_maps_to_429_with_retry_after_header() {
        let err = AppError(DomainError::RateLimited {
            retry_after_secs: Some(5),
        });

        let response = err.into_response();

        assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
        assert_eq!(
            response
                .headers()
                .get(axum::http::header::RETRY_AFTER)
                .expect("Retry-After header should be set"),
            "5"
        );
    }
}
