//! Integration tests for `AnthropicProvider` against a mocked Anthropic HTTP API.
//!
//! Follows the exact wiremock harness pattern established by `openai_provider_test.rs`:
//! stand up a `wiremock::MockServer` in place of `https://api.anthropic.com`, construct
//! the provider with `base_url` pointed at the mock server via explicit config injection
//! (never `std::env::set_var`/`remove_var`), and assert on the `DomainError` mapping for
//! each HTTP outcome.

use std::sync::Arc;
use std::time::Duration;

use llm_gateway::adapters::outbound::anthropic::AnthropicProvider;
use llm_gateway::config::{HttpClientConfig, ProviderConfig};
use llm_gateway::domain::error::DomainError;
use llm_gateway::domain::model::{ChatMessage, CompletionRequest, Role};
use llm_gateway::ports::outbound::key_store::KeyStore;
use llm_gateway::ports::outbound::provider::LLMProvider;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

/// `KeyStore` stub that always resolves a fixed dummy key.
///
/// The wiremock tests never hit a real API, so the key's value is irrelevant — only
/// that `AnthropicProvider::complete` can resolve *some* key before sending the request.
struct StubKeyStore;

impl KeyStore for StubKeyStore {
    fn get_key(&self, _provider: &str) -> Result<String, DomainError> {
        Ok("test-key".to_string())
    }
}

/// Builds an `HttpClientConfig` tuned for fast, deterministic tests: `max_retries: 0`
/// so a `429`/`5xx` response is asserted on directly instead of being retried first
/// (retry behavior is covered by `adapters/outbound/http_retry.rs`'s own unit tests,
/// not here), and short timeouts so a real failure doesn't hang the suite.
fn fast_http_config() -> HttpClientConfig {
    HttpClientConfig {
        connect_timeout: Duration::from_millis(500),
        request_timeout: Duration::from_millis(500),
        max_retries: 0,
        retry_base_delay: Duration::from_millis(10),
    }
}

/// Builds a minimal `CompletionRequest` for a given model name.
fn make_request(model: &str) -> CompletionRequest {
    CompletionRequest {
        model: model.to_string(),
        messages: vec![
            ChatMessage {
                role: Role::System,
                content: "You are helpful.".to_string().into(),
            },
            ChatMessage {
                role: Role::User,
                content: "hello".to_string().into(),
            },
        ],
        temperature: None,
        max_tokens: None,
    }
}

/// Constructs an `AnthropicProvider` pointed at `mock_server` via config injection.
fn provider_for(mock_server: &MockServer, http: HttpClientConfig) -> AnthropicProvider {
    AnthropicProvider::new(
        Arc::new(StubKeyStore),
        http,
        ProviderConfig {
            base_url: mock_server.uri(),
        },
    )
    .expect("AnthropicProvider::new should succeed with a valid config")
}

#[tokio::test]
async fn test_complete_success_maps_response_fields() {
    let mock_server = MockServer::start().await;
    let fixture = serde_json::json!({
        "id": "msg_abc123",
        "type": "message",
        "role": "assistant",
        "model": "claude-opus-4-6",
        "content": [{"type": "text", "text": "Hi there!"}],
        "stop_reason": "end_turn",
        "usage": {"input_tokens": 10, "output_tokens": 4},
    });
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&fixture))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let resp = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect("complete should succeed on a 200 response");

    assert_eq!(resp.id, "msg_abc123");
    assert_eq!(resp.model, "claude-opus-4-6");
    assert_eq!(resp.choices.len(), 1);
    assert_eq!(resp.choices[0].message.content.as_text(), "Hi there!");
    assert_eq!(resp.choices[0].finish_reason, "end_turn");
    assert_eq!(resp.usage.prompt_tokens, 10);
    assert_eq!(resp.usage.completion_tokens, 4);
    assert_eq!(resp.usage.total_tokens, 14);
}

#[tokio::test]
async fn test_complete_request_shape_hoists_system_and_always_sets_max_tokens() {
    let mock_server = MockServer::start().await;
    let fixture = serde_json::json!({
        "id": "msg_abc123",
        "model": "claude-opus-4-6",
        "content": [{"type": "text", "text": "Hi!"}],
        "stop_reason": "end_turn",
        "usage": {"input_tokens": 1, "output_tokens": 1},
    });
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&fixture))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    // `make_request` includes a system message and omits `max_tokens`, exercising both
    // the system-hoisting and max_tokens-defaulting behavior in the same request.
    provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect("complete should succeed on a 200 response");

    let received = mock_server
        .received_requests()
        .await
        .expect("request recording should be enabled by default");
    assert_eq!(received.len(), 1);

    let body: serde_json::Value =
        serde_json::from_slice(&received[0].body).expect("request body should be valid JSON");

    assert_eq!(
        body["system"].as_str(),
        Some("You are helpful."),
        "expected the system message to be hoisted to the top-level `system` field"
    );
    assert!(
        body["max_tokens"].is_number(),
        "expected `max_tokens` to always be present, got: {body}"
    );

    let messages = body["messages"]
        .as_array()
        .expect("`messages` should be an array");
    assert!(
        messages.iter().all(|m| m["role"] != "system"),
        "expected no `system`-role entry inside `messages`, got: {messages:?}"
    );
    assert_eq!(messages.len(), 1);
    assert_eq!(messages[0]["role"], "user");
}

#[tokio::test]
async fn test_complete_unauthorized_maps_to_provider_error_with_message() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(ResponseTemplate::new(401).set_body_json(serde_json::json!({
            "type": "error",
            "error": {"type": "authentication_error", "message": "invalid x-api-key"}
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect_err("a 401 response should be an error");

    match err {
        DomainError::ProviderError { message, .. } => {
            assert!(
                message.contains("invalid x-api-key"),
                "expected the Anthropic error message to be surfaced, got: {message}"
            );
        }
        other => panic!("expected DomainError::ProviderError, got {other:?}"),
    }
}

#[tokio::test]
async fn test_complete_rate_limited_maps_to_rate_limited_with_retry_after() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(
            ResponseTemplate::new(429)
                .insert_header("Retry-After", "12")
                .set_body_json(serde_json::json!({
                    "type": "error",
                    "error": {"type": "rate_limit_error", "message": "Rate limit exceeded"}
                })),
        )
        .mount(&mock_server)
        .await;

    // max_retries: 0 means the provider does not retry before surfacing this response,
    // so wiremock only needs to serve the request once.
    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect_err("a 429 response should be an error");

    assert!(matches!(
        err,
        DomainError::RateLimited {
            retry_after_secs: Some(12)
        }
    ));
}

#[tokio::test]
async fn test_complete_server_error_maps_to_provider_error() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(ResponseTemplate::new(500).set_body_json(serde_json::json!({
            "type": "error",
            "error": {"type": "api_error", "message": "Internal server error"}
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect_err("a 500 response should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_malformed_success_body_maps_to_provider_error_without_panicking() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        .respond_with(ResponseTemplate::new(200).set_body_string("this is not valid JSON"))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect_err("a 200 response with an unparseable body should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_slow_response_maps_to_timeout() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/messages"))
        // Delay is much longer than the configured request timeout below, with a wide
        // margin so the assertion does not flake under CI scheduling jitter.
        .respond_with(ResponseTemplate::new(200).set_delay(Duration::from_secs(5)))
        .mount(&mock_server)
        .await;

    let short_timeout_config = HttpClientConfig {
        connect_timeout: Duration::from_millis(200),
        request_timeout: Duration::from_millis(200),
        max_retries: 0,
        retry_base_delay: Duration::from_millis(10),
    };
    let provider = provider_for(&mock_server, short_timeout_config);

    let err = provider
        .complete(&make_request("claude-opus-4-6"))
        .await
        .expect_err("a response slower than the configured timeout should be an error");

    assert!(matches!(err, DomainError::Timeout));
}
