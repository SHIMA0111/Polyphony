use std::sync::Arc;

use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;

use crate::domain::error::DomainError;
use crate::ports::inbound::completion::CompletionUseCase;

use super::request::CompletionRequestDto;
use super::response::{CompletionResponseDto, ModelsResponseDto, ModelInfoDto};

/// アプリケーション共有状態。
pub type AppState = Arc<dyn CompletionUseCase>;

/// ヘルスチェックエンドポイント。
///
/// `GET /health` — サービスの生存確認。
pub async fn health() -> impl IntoResponse {
    Json(serde_json::json!({"status": "ok"}))
}

/// モデル一覧エンドポイント。
///
/// `GET /models` — 全プロバイダーの利用可能モデルを返す。
pub async fn list_models(State(service): State<AppState>) -> impl IntoResponse {
    let models = service
        .list_models()
        .into_iter()
        .map(ModelInfoDto::from)
        .collect();
    Json(ModelsResponseDto { models })
}

/// チャット補完エンドポイント。
///
/// `POST /completions` — LLMにチャット補完を要求する。
pub async fn complete(
    State(service): State<AppState>,
    Json(dto): Json<CompletionRequestDto>,
) -> Result<impl IntoResponse, AppError> {
    let req = dto.into_domain()?;
    let resp = service.complete(req).await?;
    Ok(Json(CompletionResponseDto::from(resp)))
}

/// ドメインエラーをHTTPレスポンスに変換するラッパー。
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
