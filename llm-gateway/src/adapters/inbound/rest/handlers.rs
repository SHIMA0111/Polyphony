use std::sync::Arc;

use axum::Json;
use axum::extract::State;
use axum::http::{HeaderValue, StatusCode};
use axum::response::IntoResponse;

use crate::domain::error::DomainError;
use crate::ports::inbound::completion::CompletionUseCase;

use super::request::{CompletionRequestDto, TokenEstimateRequestDto};
use super::response::{
    CompletionResponseDto, ModelInfoDto, ModelsResponseDto, TokenEstimateResponseDto,
};

/// Shared application state.
pub type AppState = Arc<dyn CompletionUseCase>;

/// Health check endpoint.
///
/// `GET /health` — Returns service liveness status. Always `200` once the process is
/// up; unlike `GET /ready`, it does not check whether dependencies (e.g. provider API
/// keys) are actually usable.
pub async fn health() -> impl IntoResponse {
    Json(serde_json::json!({"status": "ok"}))
}

/// Readiness check endpoint.
///
/// `GET /ready` — Returns whether the gateway's dependencies are actually usable, by
/// calling `CompletionUseCase::readiness`. Distinct from `GET /health`: a process can
/// be alive (`/health` → `200`) while not ready to serve completions (`/ready` →
/// `503`), e.g. when a registered provider's API key is missing.
///
/// # Returns
/// `200 {"status":"ready"}` when `readiness()` succeeds, `503
/// {"status":"not_ready","error":...}` otherwise.
pub async fn ready(State(service): State<AppState>) -> impl IntoResponse {
    match service.readiness() {
        Ok(()) => (StatusCode::OK, Json(serde_json::json!({"status": "ready"}))).into_response(),
        Err(e) => (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(serde_json::json!({"status": "not_ready", "error": e.to_string()})),
        )
            .into_response(),
    }
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

/// Token estimation endpoint.
///
/// `POST /tokens/estimate` — Returns an approximate token count for a list of chat
/// messages, computed by the character-based heuristic in `domain::token_estimator`.
/// Unlike `complete`, this call is infallible once the DTO is converted: it never
/// dispatches to a provider, so there is no `ModelNotFound` path; the only error is a
/// `400` from an unrecognized role string during DTO conversion.
///
/// # Arguments
/// * `service` — Shared `CompletionUseCase` implementation, injected via Axum's
///   `State` extractor.
/// * `dto` — Request body, deserialized from JSON into `TokenEstimateRequestDto`.
///
/// # Returns
/// `200` with a `TokenEstimateResponseDto` JSON body on success.
///
/// # Errors
/// Returns `AppError` (mapped to `400 Bad Request`) if `dto.into_domain()` fails to
/// convert an unrecognized role string into a domain `Role`.
pub async fn estimate_tokens(
    State(service): State<AppState>,
    Json(dto): Json<TokenEstimateRequestDto>,
) -> Result<impl IntoResponse, AppError> {
    let req = dto.into_domain()?;
    let resp = service.estimate_tokens(req);
    Ok(Json(TokenEstimateResponseDto::from(resp)))
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
