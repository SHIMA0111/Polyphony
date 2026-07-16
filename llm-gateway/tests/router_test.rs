//! HTTP-level integration tests for the assembled `axum::Router`.
//!
//! This is the canonical harness pattern for exercising `llm-gateway`'s REST surface:
//! build the router via `build_router`, drive it through `tower::util::ServiceExt::oneshot`
//! with a real `http::Request`, and assert on the real `http::StatusCode` and JSON body —
//! no bound TCP port, no real provider calls. A `StubUseCase` test double (implementing
//! `CompletionUseCase` directly, one layer above the provider level) supplies deterministic
//! success/error outcomes keyed off the requested model name.
//!
//! Later steps that add new REST-observable behavior (step 25 Anthropic, step 26 Gemini,
//! step 43 streaming) should extend this file with new cases in this same style rather
//! than inventing a new mocking approach.

use std::sync::{Arc, Mutex};

use axum::Router;
use axum::body::Body;
use axum::http::{Request, StatusCode};
use futures::future::BoxFuture;
use futures::stream::BoxStream;
use http_body_util::BodyExt;
use tower::util::ServiceExt;

use llm_gateway::adapters::inbound::rest::handlers::AppState;
use llm_gateway::adapters::inbound::rest::router::build_router;
use llm_gateway::domain::error::DomainError;
use llm_gateway::domain::model::{
    ChatMessage, Choice, CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo, Role,
    TokenEstimateRequest, TokenEstimateResponse, Usage,
};
use llm_gateway::ports::inbound::completion::CompletionUseCase;

/// Model id that `StubUseCase::complete` treats as known, returning a canned success
/// response. Any other model name (except the sentinel names below) is treated as
/// unknown, mirroring `CompletionService::find_provider` returning `None`.
const KNOWN_MODEL: &str = "stub-model";
/// Sentinel model name that makes `StubUseCase::complete` return `DomainError::Timeout`.
const TIMEOUT_MODEL: &str = "timeout-model";
/// Sentinel model name that makes `StubUseCase::complete` return `DomainError::KeyNotFound`.
const KEY_NOT_FOUND_MODEL: &str = "keynotfound-model";
/// Sentinel model name that makes `StubUseCase::complete` return `DomainError::ProviderError`.
const PROVIDER_ERROR_MODEL: &str = "providererror-model";
/// Sentinel model name that makes `StubUseCase::complete` return `DomainError::RateLimited`.
const RATE_LIMITED_MODEL: &str = "ratelimited-model";

/// Scripted outcome for `StubUseCase::stream`, set via
/// `StubUseCase::with_stream_error`/`with_stream_chunks`.
///
/// Wrapped in a `Mutex<Option<_>>` (rather than stored directly) so `StreamScript` need
/// not be `Clone` — each test drives the router's `/completions/stream` endpoint at
/// most once, so a single `Option::take()` per `StubUseCase` is sufficient.
enum StreamScript {
    /// `CompletionUseCase::stream` itself returns this error before any chunk is
    /// produced (e.g. `DomainError::ModelNotFound`), proving a pre-stream failure still
    /// surfaces as a normal non-SSE HTTP error response.
    Fails(DomainError),
    /// `CompletionUseCase::stream` succeeds, yielding exactly this scripted sequence of
    /// chunks/errors (mirroring what a real provider's stream would produce).
    Yields(Vec<Result<CompletionChunk, DomainError>>),
}

/// Test double for `CompletionUseCase`, exercised through real HTTP via `oneshot`.
///
/// Unlike `domain::service::CompletionService`'s `MockProvider` (used by pure unit
/// tests one layer below), this stub sits directly behind the router's `AppState`, so
/// it never touches provider dispatch logic — it only needs to reproduce the
/// `DomainError` → HTTP status mapping surface this test module exercises.
struct StubUseCase {
    /// Controls the `GET /ready` response: `Ok(())` reports ready, `Err` reports not ready.
    ready: Result<(), ()>,
    /// Scripted outcome for `stream()`. Defaults to a fixed provider error (mirroring
    /// the pre-Step-43 stub behavior) for tests that never call `/completions/stream`.
    stream_script: Mutex<Option<StreamScript>>,
}

impl StubUseCase {
    /// Builds a `StubUseCase` that reports ready (`GET /ready` → `200`).
    fn new() -> Self {
        Self {
            ready: Ok(()),
            stream_script: Mutex::new(None),
        }
    }

    /// Builds a `StubUseCase` that reports not ready (`GET /ready` → `503`).
    fn not_ready() -> Self {
        Self {
            ready: Err(()),
            stream_script: Mutex::new(None),
        }
    }

    /// Configures `stream()` to fail outright with `err`, before any chunk is produced.
    fn with_stream_error(self, err: DomainError) -> Self {
        *self.stream_script.lock().unwrap() = Some(StreamScript::Fails(err));
        self
    }

    /// Configures `stream()` to succeed and yield exactly `chunks`.
    fn with_stream_chunks(self, chunks: Vec<Result<CompletionChunk, DomainError>>) -> Self {
        *self.stream_script.lock().unwrap() = Some(StreamScript::Yields(chunks));
        self
    }
}

impl CompletionUseCase for StubUseCase {
    fn complete(
        &self,
        req: CompletionRequest,
    ) -> BoxFuture<'_, Result<CompletionResponse, DomainError>> {
        Box::pin(async move {
            if req.messages.is_empty() {
                return Err(DomainError::InvalidRequest(
                    "messages must not be empty".to_string(),
                ));
            }

            match req.model.as_str() {
                KNOWN_MODEL => Ok(CompletionResponse {
                    id: "stub-completion-id".to_string(),
                    model: KNOWN_MODEL.to_string(),
                    choices: vec![Choice {
                        index: 0,
                        message: ChatMessage {
                            role: Role::Assistant,
                            content: "stub response".to_string().into(),
                        },
                        finish_reason: "stop".to_string(),
                    }],
                    usage: Usage {
                        prompt_tokens: 3,
                        completion_tokens: 2,
                        total_tokens: 5,
                    },
                }),
                TIMEOUT_MODEL => Err(DomainError::Timeout),
                KEY_NOT_FOUND_MODEL => Err(DomainError::KeyNotFound("stub".to_string())),
                PROVIDER_ERROR_MODEL => Err(DomainError::provider_error("stub provider failure")),
                RATE_LIMITED_MODEL => Err(DomainError::RateLimited {
                    retry_after_secs: Some(7),
                }),
                other => Err(DomainError::ModelNotFound(other.to_string())),
            }
        })
    }

    fn list_models(&self) -> BoxFuture<'_, Vec<ModelInfo>> {
        Box::pin(async move {
            vec![ModelInfo {
                id: KNOWN_MODEL.to_string(),
                name: "Stub Model".to_string(),
                provider: "stub".to_string(),
                owned_by: "stub".to_string(),
                context_window: None,
                pricing: None,
                supports_image_input: None,
            }]
        })
    }

    fn stream(
        &self,
        _req: CompletionRequest,
    ) -> BoxFuture<'_, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>>
    {
        let script = self.stream_script.lock().unwrap().take();
        Box::pin(async move {
            match script {
                Some(StreamScript::Fails(err)) => Err(err),
                Some(StreamScript::Yields(chunks)) => Ok(Box::pin(futures::stream::iter(chunks))
                    as BoxStream<'static, Result<CompletionChunk, DomainError>>),
                None => Err(DomainError::provider_error("streaming not exercised here")),
            }
        })
    }

    fn readiness(&self) -> Result<(), DomainError> {
        self.ready
            .map_err(|()| DomainError::KeyNotFound("stub".to_string()))
    }

    fn estimate_tokens(&self, req: TokenEstimateRequest) -> TokenEstimateResponse {
        // Delegates to the real heuristic (same as `CompletionService`) so this test
        // double exercises the actual estimation behavior through the HTTP layer,
        // rather than a canned value.
        TokenEstimateResponse {
            model: req.model,
            estimated_tokens: llm_gateway::domain::token_estimator::estimate_tokens(&req.messages),
        }
    }
}

/// Builds the router under test wired to a fresh `StubUseCase`.
fn test_router(stub: StubUseCase) -> Router {
    let state: AppState = Arc::new(stub);
    build_router(state)
}

/// Drains a response body into a parsed `serde_json::Value`.
async fn body_json(response: axum::response::Response) -> serde_json::Value {
    let bytes = response.into_body().collect().await.unwrap().to_bytes();
    serde_json::from_slice(&bytes).expect("response body should be valid JSON")
}

/// Builds a `POST /completions` request with a JSON body for the given model and message.
fn completions_request(model: &str, message: &str) -> Request<Body> {
    let body = serde_json::json!({
        "model": model,
        "messages": [{"role": "user", "content": message}],
    });
    Request::builder()
        .method("POST")
        .uri("/completions")
        .header("content-type", "application/json")
        .body(Body::from(body.to_string()))
        .unwrap()
}

#[tokio::test]
async fn test_health_returns_ok_status_body() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(
            Request::builder()
                .method("GET")
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    let body = body_json(response).await;
    assert_eq!(body, serde_json::json!({"status": "ok"}));
}

#[tokio::test]
async fn test_ready_returns_ok_when_use_case_reports_ready() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(
            Request::builder()
                .method("GET")
                .uri("/ready")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    let body = body_json(response).await;
    assert_eq!(body, serde_json::json!({"status": "ready"}));
}

#[tokio::test]
async fn test_ready_returns_service_unavailable_when_use_case_reports_not_ready() {
    let router = test_router(StubUseCase::not_ready());

    let response = router
        .oneshot(
            Request::builder()
                .method("GET")
                .uri("/ready")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);
    let body = body_json(response).await;
    assert_eq!(body["status"], "not_ready");
    assert!(body["error"].is_string());
}

#[tokio::test]
async fn test_list_models_returns_aggregated_models() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(
            Request::builder()
                .method("GET")
                .uri("/models")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    let body = body_json(response).await;
    let models = body["models"]
        .as_array()
        .expect("models should be an array");
    assert_eq!(models.len(), 1);
    assert_eq!(models[0]["id"], KNOWN_MODEL);
    assert_eq!(models[0]["name"], "Stub Model");
    assert_eq!(models[0]["provider"], "stub");
    assert_eq!(models[0]["owned_by"], "stub");
}

#[tokio::test]
async fn test_completions_happy_path_returns_ok() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request(KNOWN_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    let body = body_json(response).await;
    assert_eq!(body["id"], "stub-completion-id");
    assert_eq!(body["model"], KNOWN_MODEL);
    assert_eq!(body["choices"][0]["message"]["content"], "stub response");
    assert_eq!(body["usage"]["total_tokens"], 5);
}

#[tokio::test]
async fn test_completions_unknown_model_returns_not_found() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request("no-such-model", "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn test_completions_empty_messages_returns_bad_request() {
    let router = test_router(StubUseCase::new());

    let body = serde_json::json!({"model": KNOWN_MODEL, "messages": []});
    let request = Request::builder()
        .method("POST")
        .uri("/completions")
        .header("content-type", "application/json")
        .body(Body::from(body.to_string()))
        .unwrap();

    let response = router.oneshot(request).await.unwrap();

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

/// A body that is syntactically valid JSON but does not deserialize into
/// `CompletionRequestDto` (missing the required `model`/`messages` fields) is rejected
/// by axum's `Json` extractor *before* the handler ever runs. This is axum's
/// `JsonRejection::JsonDataError` case, which axum maps to `422 UNPROCESSABLE_ENTITY` —
/// distinct from the handler's own `400 BAD_REQUEST` for a domain-level
/// `DomainError::InvalidRequest` (e.g. the empty-`messages` case above). Both are
/// "the request was rejected", but at different layers, hence the different status.
#[tokio::test]
async fn test_completions_malformed_body_returns_unprocessable_entity() {
    let router = test_router(StubUseCase::new());

    let request = Request::builder()
        .method("POST")
        .uri("/completions")
        .header("content-type", "application/json")
        .body(Body::from(r#"{"not_a_valid_field": true}"#))
        .unwrap();

    let response = router.oneshot(request).await.unwrap();

    assert_eq!(response.status(), StatusCode::UNPROCESSABLE_ENTITY);
}

#[tokio::test]
async fn test_completions_timeout_returns_gateway_timeout() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request(TIMEOUT_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::GATEWAY_TIMEOUT);
}

#[tokio::test]
async fn test_completions_key_not_found_returns_internal_server_error() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request(KEY_NOT_FOUND_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
}

#[tokio::test]
async fn test_completions_provider_error_returns_bad_gateway() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request(PROVIDER_ERROR_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::BAD_GATEWAY);
}

#[tokio::test]
async fn test_completions_rate_limited_returns_too_many_requests_with_retry_after() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(completions_request(RATE_LIMITED_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
    assert_eq!(
        response
            .headers()
            .get(axum::http::header::RETRY_AFTER)
            .expect("Retry-After header should be set"),
        "7"
    );
}

/// Builds a `POST /tokens/estimate` request with a JSON body for the given model,
/// role, and message content.
fn tokens_estimate_request(model: &str, role: &str, content: &str) -> Request<Body> {
    let body = serde_json::json!({
        "model": model,
        "messages": [{"role": role, "content": content}],
    });
    Request::builder()
        .method("POST")
        .uri("/tokens/estimate")
        .header("content-type", "application/json")
        .body(Body::from(body.to_string()))
        .unwrap()
}

#[tokio::test]
async fn test_tokens_estimate_happy_path_returns_ok() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(tokens_estimate_request(KNOWN_MODEL, "user", "hello world"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    let body = body_json(response).await;
    assert_eq!(body["model"], KNOWN_MODEL);
    let estimated_tokens = body["estimated_tokens"]
        .as_u64()
        .expect("estimated_tokens should be an integer");
    assert!(estimated_tokens > 0);
}

#[tokio::test]
async fn test_tokens_estimate_unknown_role_returns_bad_request() {
    let router = test_router(StubUseCase::new());

    let response = router
        .oneshot(tokens_estimate_request(KNOWN_MODEL, "bogus-role", "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

/// Builds a `POST /completions/stream` request with a JSON body for the given model
/// and message.
fn stream_request(model: &str, message: &str) -> Request<Body> {
    let body = serde_json::json!({
        "model": model,
        "messages": [{"role": "user", "content": message}],
    });
    Request::builder()
        .method("POST")
        .uri("/completions/stream")
        .header("content-type", "application/json")
        .body(Body::from(body.to_string()))
        .unwrap()
}

/// Drains a response body into a UTF-8 string.
async fn body_text(response: axum::response::Response) -> String {
    let bytes = response.into_body().collect().await.unwrap().to_bytes();
    String::from_utf8(bytes.to_vec()).expect("SSE body should be valid UTF-8")
}

/// Builds a minimal `CompletionChunk` for use in a scripted `StubUseCase` stream.
fn make_chunk(delta: &str) -> CompletionChunk {
    CompletionChunk {
        id: "stub-chunk-id".to_string(),
        model: KNOWN_MODEL.to_string(),
        delta: Some(delta.to_string()),
        finish_reason: None,
        usage: None,
    }
}

#[tokio::test]
async fn test_completions_stream_happy_path_returns_sse_body_with_done_sentinel() {
    let stub = StubUseCase::new()
        .with_stream_chunks(vec![Ok(make_chunk("Hi")), Ok(make_chunk(" there!"))]);
    let router = test_router(stub);

    let response = router
        .oneshot(stream_request(KNOWN_MODEL, "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(
        response
            .headers()
            .get(axum::http::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok()),
        Some("text/event-stream")
    );

    let body = body_text(response).await;
    let first_data_pos = body
        .find("data:")
        .expect("body should contain at least one data: frame");
    let second_data_pos = body[first_data_pos + 1..]
        .find("data:")
        .map(|i| i + first_data_pos + 1)
        .expect("body should contain a second data: frame");

    assert!(
        body[first_data_pos..second_data_pos].contains("\"delta\":\"Hi\""),
        "expected the first chunk's delta in the body, got: {body}"
    );
    assert!(
        body.contains("\"delta\":\" there!\""),
        "expected the second chunk's delta in the body, got: {body}"
    );
    assert!(
        body.trim_end().ends_with("data: [DONE]"),
        "expected the body to end with the [DONE] sentinel, got: {body}"
    );
}

#[tokio::test]
async fn test_completions_stream_pre_stream_error_returns_non_sse_not_found() {
    // `StubUseCase::stream` returns `Err` before any chunk is produced when the model
    // is unrecognized — reusing `no-such-model` (not `KNOWN_MODEL`) is enough on its
    // own via the default `stream_script` fallback, but this test is explicit about
    // the intended failure so it stays correct even if that default ever changes.
    let stub = StubUseCase::new()
        .with_stream_error(DomainError::ModelNotFound("no-such-model".to_string()));
    let router = test_router(stub);

    let response = router
        .oneshot(stream_request("no-such-model", "hello"))
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    assert_ne!(
        response
            .headers()
            .get(axum::http::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok()),
        Some("text/event-stream"),
        "a pre-stream error should not produce an SSE response"
    );

    let body = body_json(response).await;
    assert!(body["error"].is_string());
}

#[tokio::test]
async fn test_completions_stream_mid_stream_error_yields_error_frame_but_overall_200() {
    let stub = StubUseCase::new().with_stream_chunks(vec![
        Ok(make_chunk("partial")),
        Err(DomainError::provider_error("upstream stream failed")),
    ]);
    let router = test_router(stub);

    let response = router
        .oneshot(stream_request(KNOWN_MODEL, "hello"))
        .await
        .unwrap();

    // Status/headers are committed before the mid-stream error occurs, so the overall
    // response is still 200 even though the body carries an error frame.
    assert_eq!(response.status(), StatusCode::OK);

    let body = body_text(response).await;
    assert!(
        body.contains("\"delta\":\"partial\""),
        "expected the successful first chunk in the body, got: {body}"
    );
    assert!(
        body.contains("event: error"),
        "expected an `event: error` frame for the mid-stream failure, got: {body}"
    );
    assert!(
        body.contains("upstream stream failed"),
        "expected the error message in the error frame, got: {body}"
    );

    let error_frame_pos = body.find("event: error").unwrap();
    let done_pos = body
        .find("data: [DONE]")
        .expect("body should still end with the [DONE] sentinel after a mid-stream error");
    assert!(
        error_frame_pos < done_pos,
        "the error frame should appear before the trailing [DONE] sentinel"
    );
}
