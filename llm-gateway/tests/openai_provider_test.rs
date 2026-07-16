//! Integration tests for `OpenAIProvider` against a mocked OpenAI HTTP API.
//!
//! This is the canonical harness pattern for exercising outbound provider adapters:
//! stand up a `wiremock::MockServer` in place of `https://api.openai.com`, construct the
//! provider with `base_url` pointed at the mock server via explicit config injection
//! (never `std::env::set_var`/`remove_var`, which does not scale once tests run
//! concurrently across providers), and assert on the `DomainError` mapping for each
//! HTTP outcome.
//!
//! Later provider adapters (step 25 Anthropic, step 26 Gemini) should extend this file
//! — or add a sibling file following the same construction/assertion style — rather than
//! inventing a new mocking approach.

use std::sync::Arc;
use std::time::Duration;

use llm_gateway::adapters::outbound::openai::OpenAIProvider;
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
/// that `OpenAIProvider::complete` can resolve *some* key before sending the request.
struct StubKeyStore;

impl KeyStore for StubKeyStore {
    fn get_key(&self, _provider: &str) -> Result<String, DomainError> {
        Ok("test-key".to_string())
    }
}

/// Builds an `HttpClientConfig` tuned for fast, deterministic tests: `max_retries: 0`
/// so a `429`/`5xx` response is asserted on directly instead of being retried first
/// (step 8's retry behavior is covered by `adapters/outbound/http_retry.rs`'s own unit
/// tests, not here), and short timeouts so a real failure doesn't hang the suite.
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
        messages: vec![ChatMessage {
            role: Role::User,
            content: "hello".to_string().into(),
        }],
        temperature: None,
        max_tokens: None,
    }
}

/// Constructs an `OpenAIProvider` pointed at `mock_server` via config injection.
fn provider_for(mock_server: &MockServer, http: HttpClientConfig) -> OpenAIProvider {
    OpenAIProvider::new(
        Arc::new(StubKeyStore),
        http,
        ProviderConfig {
            base_url: mock_server.uri(),
        },
    )
    .expect("OpenAIProvider::new should succeed with a valid config")
}

#[tokio::test]
async fn test_complete_success_maps_response_fields() {
    let mock_server = MockServer::start().await;
    let fixture = serde_json::json!({
        "id": "chatcmpl-abc123",
        "model": "gpt-5.2",
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": "Hi there!"},
            "finish_reason": "stop",
        }],
        "usage": {"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
    });
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&fixture))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let resp = provider
        .complete(&make_request("gpt-5.2"))
        .await
        .expect("complete should succeed on a 200 response");

    assert_eq!(resp.id, "chatcmpl-abc123");
    assert_eq!(resp.model, "gpt-5.2");
    assert_eq!(resp.choices.len(), 1);
    assert_eq!(resp.choices[0].message.content.as_text(), "Hi there!");
    assert_eq!(resp.choices[0].finish_reason, "stop");
    assert_eq!(resp.usage.prompt_tokens, 10);
    assert_eq!(resp.usage.completion_tokens, 4);
    assert_eq!(resp.usage.total_tokens, 14);
}

#[tokio::test]
async fn test_complete_unauthorized_maps_to_provider_error_with_message() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(401).set_body_json(serde_json::json!({
            "error": {"message": "Invalid API key provided"}
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gpt-5.2"))
        .await
        .expect_err("a 401 response should be an error");

    match err {
        DomainError::ProviderError { message, .. } => {
            assert!(
                message.contains("Invalid API key provided"),
                "expected the OpenAI error message to be surfaced, got: {message}"
            );
        }
        other => panic!("expected DomainError::ProviderError, got {other:?}"),
    }
}

#[tokio::test]
async fn test_complete_rate_limited_maps_to_rate_limited_with_retry_after() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(
            ResponseTemplate::new(429)
                .insert_header("Retry-After", "12")
                .set_body_json(serde_json::json!({
                    "error": {"message": "Rate limit exceeded"}
                })),
        )
        .mount(&mock_server)
        .await;

    // max_retries: 0 means the provider does not retry before surfacing this response,
    // so wiremock only needs to serve the request once.
    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gpt-5.2"))
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
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(500).set_body_json(serde_json::json!({
            "error": {"message": "Internal server error"}
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gpt-5.2"))
        .await
        .expect_err("a 500 response should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_malformed_success_body_maps_to_provider_error_without_panicking() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .respond_with(ResponseTemplate::new(200).set_body_string("this is not valid JSON"))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gpt-5.2"))
        .await
        .expect_err("a 200 response with an unparseable body should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_slow_response_maps_to_timeout() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
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
        .complete(&make_request("gpt-5.2"))
        .await
        .expect_err("a response slower than the configured timeout should be an error");

    assert!(matches!(err, DomainError::Timeout));
}

/// End-to-end Vision test: a `CompletionRequest` whose message carries mixed
/// text+image `ContentPart`s produces the exact OpenAI multimodal `content` array
/// shape on the wire. The mock only matches (and responds 200) if the outbound body
/// equals `expected_body` exactly, so a successful `complete()` call here is itself
/// the assertion that the gateway sent the expected multimodal JSON body.
#[tokio::test]
async fn test_complete_with_image_content_sends_openai_multimodal_body() {
    use llm_gateway::domain::model::{ContentPart, MessageContent};
    use wiremock::matchers::body_json;

    let mock_server = MockServer::start().await;
    let expected_body = serde_json::json!({
        "model": "gpt-5.2",
        "messages": [{
            "role": "user",
            "content": [
                {"type": "text", "text": "what is this?"},
                {"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,abcd"}},
            ],
        }],
    });
    let response_fixture = serde_json::json!({
        "id": "chatcmpl-vision-1",
        "model": "gpt-5.2",
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": "It's a cat."},
            "finish_reason": "stop",
        }],
        "usage": {"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34},
    });
    Mock::given(method("POST"))
        .and(path("/v1/chat/completions"))
        .and(body_json(&expected_body))
        .respond_with(ResponseTemplate::new(200).set_body_json(&response_fixture))
        .mount(&mock_server)
        .await;

    let req = CompletionRequest {
        model: "gpt-5.2".to_string(),
        messages: vec![ChatMessage {
            role: Role::User,
            content: MessageContent::Parts(vec![
                ContentPart::Text("what is this?".to_string()),
                ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
                ContentPart::ImageBase64 {
                    media_type: "image/png".to_string(),
                    data: "abcd".to_string(),
                },
            ]),
        }],
        temperature: None,
        max_tokens: None,
    };

    let provider = provider_for(&mock_server, fast_http_config());
    let resp = provider
        .complete(&req)
        .await
        .expect("complete should succeed once the mock's exact-body matcher accepts the request");

    assert_eq!(resp.choices[0].message.content.as_text(), "It's a cat.");
}
