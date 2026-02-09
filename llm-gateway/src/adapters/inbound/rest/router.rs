use axum::routing::{get, post};
use axum::Router;

use super::handlers::{complete, health, list_models, AppState};

/// Builds the axum router.
///
/// # Arguments
/// * `state` — Shared state implementing `CompletionUseCase`
pub fn build_router(state: AppState) -> Router {
    Router::new()
        .route("/health", get(health))
        .route("/models", get(list_models))
        .route("/completions", post(complete))
        .with_state(state)
}
