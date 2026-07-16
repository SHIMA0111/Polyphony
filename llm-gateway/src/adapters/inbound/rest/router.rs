use axum::Router;
use axum::http::Request;
use axum::routing::{get, post};
use tower::ServiceBuilder;
use tower_http::request_id::{PropagateRequestIdLayer, SetRequestIdLayer};
use tower_http::trace::{DefaultOnRequest, DefaultOnResponse, TraceLayer};
use tracing::Level;

use super::handlers::{AppState, complete, health, list_models, ready};
use super::middleware::UuidRequestId;

/// HTTP header used to correlate a request across logs and the response.
const REQUEST_ID_HEADER: &str = "x-request-id";

/// Builds the axum router.
///
/// Every request is tagged with an `X-Request-Id` (generated via `UuidRequestId` if
/// the client did not supply one), wrapped in a `tower-http` `TraceLayer` span that
/// includes the request id, method, and URI so `tracing` logs correlate to a single
/// request, and the id is propagated back onto the response.
///
/// # Arguments
/// * `state` — Shared state implementing `CompletionUseCase`.
///
/// # Returns
/// A fully assembled `Router` with `/health`, `/ready`, `/models`, and
/// `/completions` routes registered, the request-id/tracing middleware
/// layered on, and `state` bound via `with_state`, ready to be served.
pub fn build_router(state: AppState) -> Router {
    let header_name = axum::http::HeaderName::from_static(REQUEST_ID_HEADER);

    let middleware = ServiceBuilder::new()
        .layer(SetRequestIdLayer::new(header_name.clone(), UuidRequestId))
        .layer(
            TraceLayer::new_for_http()
                .make_span_with(move |request: &Request<axum::body::Body>| {
                    let request_id = request
                        .headers()
                        .get(&header_name)
                        .and_then(|v| v.to_str().ok())
                        .unwrap_or("");
                    tracing::info_span!(
                        "http_request",
                        request_id = %request_id,
                        method = %request.method(),
                        uri = %request.uri(),
                    )
                })
                // Log request/response events at INFO (tower-http defaults to DEBUG),
                // so every request produces a correlated JSON log line by default.
                .on_request(DefaultOnRequest::new().level(Level::INFO))
                .on_response(DefaultOnResponse::new().level(Level::INFO)),
        )
        .layer(PropagateRequestIdLayer::new(
            axum::http::HeaderName::from_static(REQUEST_ID_HEADER),
        ));

    Router::new()
        .route("/health", get(health))
        .route("/ready", get(ready))
        .route("/models", get(list_models))
        .route("/completions", post(complete))
        .layer(middleware)
        .with_state(state)
}
