//! Integration tests for `GeminiProvider` against a mocked Google Generative Language
//! API.
//!
//! Follows the `wiremock` harness pattern established in `tests/openai_provider_test.rs`
//! (Step 14): stand up a `wiremock::MockServer` in place of
//! `https://generativelanguage.googleapis.com`, construct the provider with `base_url`
//! pointed at the mock server via explicit config injection, and assert on the
//! `DomainError` mapping for each HTTP outcome. No real network calls are made.

use std::sync::Arc;
use std::time::Duration;

use futures::StreamExt;
use llm_gateway::adapters::outbound::gemini::GeminiProvider;
use llm_gateway::adapters::outbound::openai::OpenAIProvider;
use llm_gateway::config::{HttpClientConfig, ProviderConfig};
use llm_gateway::domain::error::DomainError;
use llm_gateway::domain::model::{ChatMessage, CompletionRequest, Role};
use llm_gateway::ports::outbound::key_store::KeyStore;
use llm_gateway::ports::outbound::provider::LLMProvider;
use wiremock::matchers::{body_partial_json, method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

/// `KeyStore` stub that always resolves a fixed dummy key.
///
/// The wiremock tests never hit a real API, so the key's value is irrelevant — only
/// that `GeminiProvider::complete` can resolve *some* key before sending the request.
struct StubKeyStore;

impl KeyStore for StubKeyStore {
    fn get_key(&self, _provider: &str) -> Result<String, DomainError> {
        Ok("test-key".to_string())
    }
}

/// Builds an `HttpClientConfig` tuned for fast, deterministic tests: `max_retries: 0`
/// so a `429`/`5xx` response is asserted on directly instead of being retried first,
/// and short timeouts so a real failure doesn't hang the suite.
fn fast_http_config() -> HttpClientConfig {
    HttpClientConfig {
        connect_timeout: Duration::from_millis(500),
        request_timeout: Duration::from_millis(500),
        max_retries: 0,
        retry_base_delay: Duration::from_millis(10),
    }
}

/// Builds a minimal single-turn `CompletionRequest` for a given model name.
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

/// Constructs a `GeminiProvider` pointed at `mock_server` via config injection.
fn provider_for(mock_server: &MockServer, http: HttpClientConfig) -> GeminiProvider {
    GeminiProvider::new(
        Arc::new(StubKeyStore),
        http,
        ProviderConfig {
            base_url: mock_server.uri(),
        },
    )
    .expect("GeminiProvider::new should succeed with a valid config")
}

#[tokio::test]
async fn test_complete_success_maps_response_fields() {
    let mock_server = MockServer::start().await;
    let fixture = serde_json::json!({
        "candidates": [{
            "content": {"role": "model", "parts": [{"text": "Hi there!"}]},
            "finishReason": "STOP",
            "index": 0,
        }],
        "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 4, "totalTokenCount": 14},
        "responseId": "resp-abc123",
    });
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&fixture))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let resp = provider
        .complete(&make_request("gemini-3-pro"))
        .await
        .expect("complete should succeed on a 200 response");

    assert_eq!(resp.id, "resp-abc123");
    assert_eq!(resp.model, "gemini-3-pro");
    assert_eq!(resp.choices.len(), 1);
    assert_eq!(resp.choices[0].message.content.as_text(), "Hi there!");
    assert_eq!(resp.choices[0].finish_reason, "STOP");
    assert_eq!(resp.usage.prompt_tokens, 10);
    assert_eq!(resp.usage.completion_tokens, 4);
    assert_eq!(resp.usage.total_tokens, 14);
}

#[tokio::test]
async fn test_complete_multi_turn_with_system_message_sends_system_instruction() {
    let mock_server = MockServer::start().await;
    let fixture = serde_json::json!({
        "candidates": [{
            "content": {"role": "model", "parts": [{"text": "Fine, thanks!"}]},
            "finishReason": "STOP",
            "index": 0,
        }],
        "usageMetadata": {"promptTokenCount": 20, "candidatesTokenCount": 3, "totalTokenCount": 23},
    });

    // `body_partial_json` asserts the captured request body contains at least these
    // fields — proving `systemInstruction` was sent as a top-level field, distinct
    // from `contents`, and that the system message is absent from `contents` (which
    // here holds exactly the 3 non-system turns).
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .and(body_partial_json(serde_json::json!({
            "systemInstruction": {"parts": [{"text": "You are helpful."}]},
            "contents": [
                {"role": "user", "parts": [{"text": "Hello"}]},
                {"role": "model", "parts": [{"text": "Hi!"}]},
                {"role": "user", "parts": [{"text": "How are you?"}]},
            ],
        })))
        .respond_with(ResponseTemplate::new(200).set_body_json(&fixture))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let req = CompletionRequest {
        model: "gemini-3-pro".to_string(),
        messages: vec![
            ChatMessage {
                role: Role::System,
                content: "You are helpful.".to_string().into(),
            },
            ChatMessage {
                role: Role::User,
                content: "Hello".to_string().into(),
            },
            ChatMessage {
                role: Role::Assistant,
                content: "Hi!".to_string().into(),
            },
            ChatMessage {
                role: Role::User,
                content: "How are you?".to_string().into(),
            },
        ],
        temperature: None,
        max_tokens: None,
    };

    let resp = provider
        .complete(&req)
        .await
        .expect("complete should succeed when the mock matches the expected request body");

    assert_eq!(resp.choices[0].message.content.as_text(), "Fine, thanks!");
}

#[tokio::test]
async fn test_complete_safety_blocked_maps_to_provider_error_with_block_reason() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "candidates": [],
            "promptFeedback": {"blockReason": "SAFETY"},
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gemini-3-pro"))
        .await
        .expect_err("an empty-candidates safety-blocked response should be an error");

    match err {
        DomainError::ProviderError { message, .. } => {
            assert!(
                message.contains("SAFETY"),
                "expected the block reason to be surfaced, got: {message}"
            );
        }
        other => panic!("expected DomainError::ProviderError, got {other:?}"),
    }
}

#[tokio::test]
async fn test_complete_rate_limited_maps_to_rate_limited_with_retry_after() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .respond_with(
            ResponseTemplate::new(429)
                .insert_header("Retry-After", "12")
                .set_body_json(serde_json::json!({
                    "error": {"message": "Resource has been exhausted"}
                })),
        )
        .mount(&mock_server)
        .await;

    // max_retries: 0 means the provider does not retry before surfacing this response,
    // so wiremock only needs to serve the request once.
    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gemini-3-pro"))
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
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .respond_with(ResponseTemplate::new(500).set_body_json(serde_json::json!({
            "error": {"message": "Internal error"}
        })))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gemini-3-pro"))
        .await
        .expect_err("a 500 response should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_malformed_success_body_maps_to_provider_error_without_panicking() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .respond_with(ResponseTemplate::new(200).set_body_string("this is not valid JSON"))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let err = provider
        .complete(&make_request("gemini-3-pro"))
        .await
        .expect_err("a 200 response with an unparseable body should be an error");

    assert!(matches!(err, DomainError::ProviderError { .. }));
}

#[tokio::test]
async fn test_complete_slow_response_maps_to_timeout() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
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
        .complete(&make_request("gemini-3-pro"))
        .await
        .expect_err("a response slower than the configured timeout should be an error");

    assert!(matches!(err, DomainError::Timeout));
}

/// End-to-end Vision test: a `CompletionRequest` whose message carries mixed
/// text+image `ContentPart`s produces the exact Gemini multimodal `parts` array shape
/// on the wire (`text`, `fileData`, and `inlineData` parts). The mock only matches
/// (and responds 200) if the outbound body equals `expected_body` exactly, so a
/// successful `complete()` call here is itself the assertion that the gateway sent the
/// expected multimodal JSON body.
#[tokio::test]
async fn test_complete_with_image_content_sends_gemini_multimodal_body() {
    use llm_gateway::domain::model::{ContentPart, MessageContent};
    use wiremock::matchers::body_json;

    let mock_server = MockServer::start().await;
    let expected_body = serde_json::json!({
        "contents": [{
            "role": "user",
            "parts": [
                {"text": "what is this?"},
                {"fileData": {"fileUri": "https://example.com/cat.png"}},
                {"inlineData": {"mimeType": "image/png", "data": "abcd"}},
            ],
        }],
    });
    let response_fixture = serde_json::json!({
        "candidates": [{
            "content": {"role": "model", "parts": [{"text": "It's a cat."}]},
            "finishReason": "STOP",
            "index": 0,
        }],
        "usageMetadata": {"promptTokenCount": 30, "candidatesTokenCount": 4, "totalTokenCount": 34},
        "responseId": "resp-vision-1",
    });
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:generateContent"))
        .and(body_json(&expected_body))
        .respond_with(ResponseTemplate::new(200).set_body_json(&response_fixture))
        .mount(&mock_server)
        .await;

    let req = CompletionRequest {
        model: "gemini-3-pro".to_string(),
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

/// Canned Gemini `streamGenerateContent` SSE body: two partial-text events followed by
/// a final event carrying `finishReason`/`usageMetadata`. No event carries a
/// `responseId`, exercising the synthetic-UUID fallback path.
const GEMINI_SSE_FIXTURE: &str = concat!(
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Hi\"}]},\"index\":0}]}\n\n",
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\" there!\"}]},\"index\":0}]}\n\n",
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":4,\"totalTokenCount\":14}}\n\n",
);

/// Same shape as [`GEMINI_SSE_FIXTURE`], but every event carries Gemini's documented
/// `responseId`, exercising the path where the wire value is preferred over the
/// synthetic UUID fallback.
const GEMINI_SSE_FIXTURE_WITH_RESPONSE_ID: &str = concat!(
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Hi\"}]},\"index\":0}],\"responseId\":\"resp-stream-1\"}\n\n",
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\" there!\"}]},\"index\":0}],\"responseId\":\"resp-stream-1\"}\n\n",
    "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":4,\"totalTokenCount\":14},\"responseId\":\"resp-stream-1\"}\n\n",
);

#[tokio::test]
async fn test_stream_success_yields_expected_chunk_sequence() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:streamGenerateContent"))
        .and(query_param("alt", "sse"))
        .respond_with(
            ResponseTemplate::new(200).set_body_raw(GEMINI_SSE_FIXTURE, "text/event-stream"),
        )
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let chunk_stream = provider
        .stream(&make_request("gemini-3-pro"))
        .await
        .expect("stream should be established on a 200 SSE response");

    let chunks: Vec<_> = chunk_stream
        .collect::<Vec<_>>()
        .await
        .into_iter()
        .map(|c| c.expect("every chunk should parse successfully"))
        .collect();

    assert_eq!(chunks.len(), 3);

    let full_text: String = chunks.iter().filter_map(|c| c.delta.clone()).collect();
    assert_eq!(full_text, "Hi there!");

    // No event carries `responseId`, so the synthetic id is generated once and reused
    // across every chunk.
    let id = chunks[0].id.clone();
    assert!(!id.is_empty());
    for chunk in &chunks {
        assert_eq!(chunk.id, id);
        assert_eq!(chunk.model, "gemini-3-pro");
    }

    let last = &chunks[2];
    assert_eq!(last.finish_reason.as_deref(), Some("STOP"));
    let usage = last.usage.as_ref().expect("final chunk should carry usage");
    assert_eq!(usage.prompt_tokens, 10);
    assert_eq!(usage.completion_tokens, 4);
    assert_eq!(usage.total_tokens, 14);
}

/// When Gemini does echo `responseId` on stream events, every yielded chunk's `id` must
/// be that wire value, not a synthetic UUID.
#[tokio::test]
async fn test_stream_success_prefers_wire_response_id_over_synthetic_uuid() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:streamGenerateContent"))
        .and(query_param("alt", "sse"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_raw(GEMINI_SSE_FIXTURE_WITH_RESPONSE_ID, "text/event-stream"),
        )
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let chunk_stream = provider
        .stream(&make_request("gemini-3-pro"))
        .await
        .expect("stream should be established on a 200 SSE response");

    let chunks: Vec<_> = chunk_stream
        .collect::<Vec<_>>()
        .await
        .into_iter()
        .map(|c| c.expect("every chunk should parse successfully"))
        .collect();

    assert_eq!(chunks.len(), 3);
    for chunk in &chunks {
        assert_eq!(chunk.id, "resp-stream-1");
    }
}

#[tokio::test]
async fn test_stream_malformed_event_yields_err_item_without_panicking() {
    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:streamGenerateContent"))
        .and(query_param("alt", "sse"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_raw("data: this is not valid JSON\n\n", "text/event-stream"),
        )
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let mut chunk_stream = provider
        .stream(&make_request("gemini-3-pro"))
        .await
        .expect("stream should be established on a 200 SSE response");

    let first = chunk_stream
        .next()
        .await
        .expect("stream should yield exactly one item for the malformed event");
    assert!(matches!(first, Err(DomainError::ProviderError { .. })));
    assert!(
        chunk_stream.next().await.is_none(),
        "stream should end after the malformed event, not continue or panic"
    );
}

#[tokio::test]
async fn test_stream_block_reason_yields_err_item_without_panicking() {
    let mock_server = MockServer::start().await;
    let fixture = "data: {\"candidates\":[],\"promptFeedback\":{\"blockReason\":\"SAFETY\"}}\n\n";
    Mock::given(method("POST"))
        .and(path("/v1beta/models/gemini-3-pro:streamGenerateContent"))
        .and(query_param("alt", "sse"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(fixture, "text/event-stream"))
        .mount(&mock_server)
        .await;

    let provider = provider_for(&mock_server, fast_http_config());
    let mut chunk_stream = provider
        .stream(&make_request("gemini-3-pro"))
        .await
        .expect("stream should be established on a 200 SSE response");

    let first = chunk_stream
        .next()
        .await
        .expect("stream should yield exactly one item for the blocked event");
    match first {
        Err(DomainError::ProviderError { message, .. }) => {
            assert!(
                message.contains("SAFETY"),
                "expected the block reason to be surfaced, got: {message}"
            );
        }
        other => panic!("expected Err(DomainError::ProviderError), got {other:?}"),
    }
    assert!(chunk_stream.next().await.is_none());
}

/// Proves that `GeminiProvider`'s hardcoded model IDs (`gemini-` prefixed) do not
/// collide with `OpenAIProvider`'s (`gpt-`/`o`-prefixed). Both providers are
/// constructed via their Step-8 config-injection constructors with a trivial
/// always-succeeding stub `KeyStore` — `.models()` is a pure/static method, so no
/// network call or real API key is involved.
#[tokio::test]
async fn test_model_ids_do_not_collide_across_providers() {
    let http = fast_http_config();

    let openai = OpenAIProvider::new(
        Arc::new(StubKeyStore),
        http.clone(),
        ProviderConfig {
            base_url: "http://unused.invalid".to_string(),
        },
    )
    .expect("OpenAIProvider::new should succeed with a valid config");
    let gemini = GeminiProvider::new(
        Arc::new(StubKeyStore),
        http,
        ProviderConfig {
            base_url: "http://unused.invalid".to_string(),
        },
    )
    .expect("GeminiProvider::new should succeed with a valid config");

    let mut ids: Vec<String> = openai.models().await.into_iter().map(|m| m.id).collect();
    ids.extend(gemini.models().await.into_iter().map(|m| m.id));

    let unique: std::collections::HashSet<&String> = ids.iter().collect();
    assert_eq!(
        unique.len(),
        ids.len(),
        "expected no duplicate model IDs across OpenAI and Gemini, got: {ids:?}"
    );
}
