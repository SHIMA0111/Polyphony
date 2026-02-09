use axum::routing::{get, post};
use axum::Router;

use super::handlers::{complete, health, list_models, AppState};

/// axumルーターを構築する。
///
/// # Arguments
/// * `state` — `CompletionUseCase` を実装した共有状態
pub fn build_router(state: AppState) -> Router {
    Router::new()
        .route("/health", get(health))
        .route("/models", get(list_models))
        .route("/completions", post(complete))
        .with_state(state)
}
