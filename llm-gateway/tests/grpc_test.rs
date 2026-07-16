//! Integration tests for the gRPC inbound adapter
//! (`llm_gateway::adapters::inbound::grpc`).
//!
//! Spins up a real `tonic` server (`serve_grpc`) backed by a `StubUseCase` test
//! double, connects a `tonic::transport::Channel` to it, and exercises
//! `CompletionService`, `ModelsService`, and the standard `grpc.health.v1.Health`
//! service end-to-end.
//!
//! `build.rs` generates server stubs only (`build_client(false)`) since the gateway
//! never calls these services as a client — so, unlike the standard `tonic-health`
//! client (which the `tonic-health` crate always ships), this test invokes
//! `CompletionService`/`ModelsService` via the low-level `tonic::client::Grpc` API
//! directly, the same machinery a generated client would use internally.
//!
//! All assertions run inside a single `#[tokio::test]` function sharing one server
//! instance/port, rather than one server per test function, so the fixed test port
//! below is never raced by cargo's default parallel test execution.

use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;

use futures::future::BoxFuture;
use futures::stream::BoxStream;
use tonic::transport::Channel;

use llm_gateway::adapters::inbound::grpc::pb::{
    ChatMessage, ChatRole, CompletionRequest, CompletionResponse, ContentPart, ContentParts,
    ImageBase64Data, ListModelsRequest, ListModelsResponse, ModelInfo as ProtoModelInfo,
    TokenEstimateRequest, TokenEstimateResponse, chat_message, content_part,
};
use llm_gateway::adapters::inbound::grpc::serve_grpc;
use llm_gateway::domain::error::DomainError;
use llm_gateway::domain::model::{self, ModelPricing};
use llm_gateway::ports::inbound::completion::CompletionUseCase;

/// Fixed high test port for the in-process gRPC server.
///
/// `serve_grpc` binds the address it is given directly (it does not report back an
/// OS-assigned ephemeral port), so a fixed high port is used here rather than
/// `127.0.0.1:0`. All assertions share a single bind of this port (see the module
/// doc comment) so there is no risk of two tests racing for it.
const TEST_GRPC_PORT: u16 = 50199;

/// Test double implementing `CompletionUseCase`, mirroring the `MockProvider` pattern
/// used in `llm_gateway::domain::service`'s unit tests.
struct StubUseCase {
    models: Vec<model::ModelInfo>,
}

impl StubUseCase {
    fn new() -> Self {
        Self {
            models: vec![model::ModelInfo {
                id: "stub-model".to_string(),
                name: "Stub Model".to_string(),
                provider: "stub".to_string(),
                owned_by: "stub".to_string(),
                context_window: Some(128_000),
                pricing: Some(ModelPricing {
                    input_price_per_million_tokens: 1.0,
                    output_price_per_million_tokens: 2.0,
                    currency: "USD".to_string(),
                }),
                supports_image_input: Some(true),
            }],
        }
    }
}

impl CompletionUseCase for StubUseCase {
    fn complete(
        &self,
        req: model::CompletionRequest,
    ) -> BoxFuture<'_, Result<model::CompletionResponse, DomainError>> {
        Box::pin(async move {
            // "vision-echo-model" echoes the first request message's content straight
            // back as the assistant's reply, so a test can assert a `MessageContent`
            // (in particular `Parts`) survives the proto round trip unchanged.
            if req.model == "vision-echo-model" {
                let content = req
                    .messages
                    .into_iter()
                    .next()
                    .map(|m| m.content)
                    .unwrap_or_else(|| model::MessageContent::Text(String::new()));
                return Ok(model::CompletionResponse {
                    id: "stub-vision-completion-id".to_string(),
                    model: req.model,
                    choices: vec![model::Choice {
                        index: 0,
                        message: model::ChatMessage {
                            role: model::Role::Assistant,
                            content,
                        },
                        finish_reason: "stop".to_string(),
                    }],
                    usage: model::Usage {
                        prompt_tokens: 3,
                        completion_tokens: 2,
                        total_tokens: 5,
                    },
                });
            }

            if req.model != "stub-model" {
                return Err(DomainError::ModelNotFound(req.model));
            }

            Ok(model::CompletionResponse {
                id: "stub-completion-id".to_string(),
                model: req.model,
                choices: vec![model::Choice {
                    index: 0,
                    message: model::ChatMessage {
                        role: model::Role::Assistant,
                        content: "stub response".to_string().into(),
                    },
                    finish_reason: "stop".to_string(),
                }],
                usage: model::Usage {
                    prompt_tokens: 3,
                    completion_tokens: 2,
                    total_tokens: 5,
                },
            })
        })
    }

    fn list_models(&self) -> BoxFuture<'_, Vec<model::ModelInfo>> {
        let models = self.models.clone();
        Box::pin(async move { models })
    }

    fn stream(
        &self,
        _req: model::CompletionRequest,
    ) -> BoxFuture<
        '_,
        Result<BoxStream<'static, Result<model::CompletionChunk, DomainError>>, DomainError>,
    > {
        Box::pin(async move {
            Err(DomainError::provider_error(
                "streaming not supported by stub use case",
            ))
        })
    }

    fn readiness(&self) -> Result<(), DomainError> {
        Ok(())
    }

    fn estimate_tokens(&self, req: model::TokenEstimateRequest) -> model::TokenEstimateResponse {
        // Not exercised by this gRPC test module (token estimation has no gRPC
        // transport yet); delegate to the real heuristic so the stub stays truthful
        // if a future gRPC estimation method is added on top of this same state.
        model::TokenEstimateResponse {
            model: req.model,
            estimated_tokens: llm_gateway::domain::token_estimator::estimate_tokens(&req.messages),
        }
    }
}

/// Starts `serve_grpc` backed by a `StubUseCase` in a background task, waits for the
/// port to accept connections, and returns a connected `Channel`.
async fn start_test_server_and_connect() -> Channel {
    let addr: SocketAddr = ([127, 0, 0, 1], TEST_GRPC_PORT).into();
    let state: Arc<dyn CompletionUseCase> = Arc::new(StubUseCase::new());

    tokio::spawn(async move {
        // The test never signals shutdown; the spawned task is simply dropped when
        // the test process exits.
        serve_grpc(state, addr, std::future::pending())
            .await
            .expect("gRPC server error");
    });

    // Poll until the server accepts connections rather than sleeping a fixed amount,
    // so the test isn't flaky under slow CI hosts.
    let endpoint = format!("http://{addr}");
    for _ in 0..50 {
        if let Ok(channel) = Channel::from_shared(endpoint.clone())
            .expect("valid endpoint")
            .connect()
            .await
        {
            return channel;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
    panic!("gRPC test server did not become ready in time");
}

/// Sends a unary `CompletionService/Complete` request without a generated client
/// stub (this crate only generates server code), using the same low-level
/// `tonic::client::Grpc` + `tonic_prost::ProstCodec` machinery a generated client
/// would use internally.
async fn call_complete(
    channel: Channel,
    request: CompletionRequest,
) -> Result<CompletionResponse, tonic::Status> {
    let mut grpc = tonic::client::Grpc::new(channel);
    grpc.ready().await.expect("channel should become ready");
    let path =
        http::uri::PathAndQuery::from_static("/polyphony.llmgateway.v1.CompletionService/Complete");
    let codec = tonic_prost::ProstCodec::<CompletionRequest, CompletionResponse>::default();
    grpc.unary(tonic::Request::new(request), path, codec)
        .await
        .map(|resp| resp.into_inner())
}

/// Sends a unary `ModelsService/ListModels` request via the same low-level path as
/// `call_complete`.
async fn call_list_models(channel: Channel) -> Result<ListModelsResponse, tonic::Status> {
    let mut grpc = tonic::client::Grpc::new(channel);
    grpc.ready().await.expect("channel should become ready");
    let path =
        http::uri::PathAndQuery::from_static("/polyphony.llmgateway.v1.ModelsService/ListModels");
    let codec = tonic_prost::ProstCodec::<ListModelsRequest, ListModelsResponse>::default();
    grpc.unary(tonic::Request::new(ListModelsRequest {}), path, codec)
        .await
        .map(|resp| resp.into_inner())
}

/// Sends a unary `CompletionService/EstimateTokens` request via the same low-level
/// path as `call_complete`.
async fn call_estimate_tokens(
    channel: Channel,
    request: TokenEstimateRequest,
) -> Result<TokenEstimateResponse, tonic::Status> {
    let mut grpc = tonic::client::Grpc::new(channel);
    grpc.ready().await.expect("channel should become ready");
    let path = http::uri::PathAndQuery::from_static(
        "/polyphony.llmgateway.v1.CompletionService/EstimateTokens",
    );
    let codec = tonic_prost::ProstCodec::<TokenEstimateRequest, TokenEstimateResponse>::default();
    grpc.unary(tonic::Request::new(request), path, codec)
        .await
        .map(|resp| resp.into_inner())
}

#[tokio::test]
async fn test_grpc_completion_models_and_health_services() {
    let channel = start_test_server_and_connect().await;

    // --- CompletionService/Complete: happy path ---
    let request = CompletionRequest {
        model: "stub-model".to_string(),
        messages: vec![ChatMessage {
            role: ChatRole::User as i32,
            content: Some(chat_message::Content::Text("hello".to_string())),
        }],
        temperature: None,
        max_tokens: None,
    };
    let response = call_complete(channel.clone(), request)
        .await
        .expect("Complete should succeed for a known model");

    assert_eq!(response.id, "stub-completion-id");
    assert_eq!(response.model, "stub-model");
    assert_eq!(response.choices.len(), 1);
    let choice = &response.choices[0];
    assert_eq!(choice.finish_reason, "stop");
    let message = choice.message.as_ref().expect("message should be set");
    assert_eq!(message.role, ChatRole::Assistant as i32);
    assert_eq!(
        message.content,
        Some(chat_message::Content::Text("stub response".to_string()))
    );
    let usage = response.usage.expect("usage should be set");
    assert_eq!(usage.prompt_tokens, 3);
    assert_eq!(usage.completion_tokens, 2);
    assert_eq!(usage.total_tokens, 5);

    // --- CompletionService/Complete: unknown model maps to NotFound ---
    let bad_request = CompletionRequest {
        model: "nonexistent-model".to_string(),
        messages: vec![ChatMessage {
            role: ChatRole::User as i32,
            content: Some(chat_message::Content::Text("hello".to_string())),
        }],
        temperature: None,
        max_tokens: None,
    };
    let status = call_complete(channel.clone(), bad_request)
        .await
        .expect_err("Complete should fail for an unknown model");
    assert_eq!(status.code(), tonic::Code::NotFound);

    // --- ModelsService/ListModels ---
    let list_response = call_list_models(channel.clone())
        .await
        .expect("ListModels should succeed");
    assert_eq!(list_response.models.len(), 1);
    let model: &ProtoModelInfo = &list_response.models[0];
    assert_eq!(model.id, "stub-model");
    assert_eq!(model.context_window, Some(128_000));
    let pricing = model.pricing.as_ref().expect("pricing should be populated");
    assert_eq!(pricing.input_price_per_million_tokens, 1.0);
    assert_eq!(pricing.output_price_per_million_tokens, 2.0);
    assert_eq!(pricing.currency, "USD");
    assert_eq!(model.supports_image_input, Some(true));

    // --- CompletionService/EstimateTokens ---
    let estimate_request = TokenEstimateRequest {
        model: "stub-model".to_string(),
        messages: vec![ChatMessage {
            role: ChatRole::User as i32,
            content: Some(chat_message::Content::Text("hello world".to_string())),
        }],
    };
    let estimate_response = call_estimate_tokens(channel.clone(), estimate_request)
        .await
        .expect("EstimateTokens should succeed");
    assert_eq!(estimate_response.model, "stub-model");
    assert!(estimate_response.estimated_tokens > 0);

    // Unlike `Complete`, an unrecognized model name does not cause an error --
    // estimation never dispatches to a provider.
    let estimate_unknown_model = TokenEstimateRequest {
        model: "totally-unknown-model".to_string(),
        messages: vec![ChatMessage {
            role: ChatRole::User as i32,
            content: Some(chat_message::Content::Text("hi".to_string())),
        }],
    };
    let estimate_unknown_response = call_estimate_tokens(channel.clone(), estimate_unknown_model)
        .await
        .expect("EstimateTokens should succeed even for an unrecognized model");
    assert_eq!(estimate_unknown_response.model, "totally-unknown-model");
    assert!(estimate_unknown_response.estimated_tokens > 0);

    // --- CompletionService/Complete: multimodal (ContentParts) round trip ---
    // Proves that a multimodal request message survives the full proto round trip:
    // CompletionRequest.messages[0].content.parts -> domain MessageContent::Parts (via
    // StubUseCase, which echoes it straight back as the assistant's reply) ->
    // CompletionResponse.choices[0].message.content.parts. Reuses the same `channel`
    // (rather than a second `start_test_server_and_connect()` call) since this test
    // module binds a single fixed port shared by every assertion in this test
    // function -- see the module doc comment.
    let vision_request = CompletionRequest {
        model: "vision-echo-model".to_string(),
        messages: vec![ChatMessage {
            role: ChatRole::User as i32,
            content: Some(chat_message::Content::Parts(ContentParts {
                parts: vec![
                    ContentPart {
                        part: Some(content_part::Part::Text("what is this?".to_string())),
                    },
                    ContentPart {
                        part: Some(content_part::Part::ImageBase64(ImageBase64Data {
                            media_type: "image/png".to_string(),
                            data: "abcd".to_string(),
                        })),
                    },
                ],
            })),
        }],
        temperature: None,
        max_tokens: None,
    };
    let vision_response = call_complete(channel.clone(), vision_request)
        .await
        .expect("Complete should succeed for the echo model");
    let vision_message = vision_response.choices[0]
        .message
        .as_ref()
        .expect("message should be set");
    match vision_message
        .content
        .as_ref()
        .expect("content should be set")
    {
        chat_message::Content::Parts(parts) => {
            assert_eq!(parts.parts.len(), 2);
            assert_eq!(
                parts.parts[0].part,
                Some(content_part::Part::Text("what is this?".to_string()))
            );
            assert_eq!(
                parts.parts[1].part,
                Some(content_part::Part::ImageBase64(ImageBase64Data {
                    media_type: "image/png".to_string(),
                    data: "abcd".to_string(),
                }))
            );
        }
        other => panic!("expected chat_message::Content::Parts, got {other:?}"),
    }

    // --- grpc.health.v1.Health/Check ---
    let mut health_client = tonic_health::pb::health_client::HealthClient::new(channel);
    let health_response = health_client
        .check(tonic_health::pb::HealthCheckRequest {
            service: "polyphony.llmgateway.v1.CompletionService".to_string(),
        })
        .await
        .expect("health check should succeed")
        .into_inner();
    assert_eq!(
        health_response.status,
        tonic_health::pb::health_check_response::ServingStatus::Serving as i32
    );
}
