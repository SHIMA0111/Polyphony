//! gRPC `CompletionService` implementation.
//!
//! Delegates to the same `Arc<dyn CompletionUseCase>` the REST `POST /completions`
//! handler uses (`adapters::inbound::rest::handlers::complete`); this adapter only
//! translates between the wire (proto) and domain shapes.

use std::sync::Arc;

use tonic::{Request, Response, Status};

use crate::domain::model as domain;
use crate::ports::inbound::completion::CompletionUseCase;

use super::convert::{domain_error_to_status, domain_role_to_proto, proto_role_to_domain};
use super::pb::completion_service_server::CompletionService;
use super::pb::{ChatMessage, Choice, CompletionRequest, CompletionResponse, Usage};

/// Tonic server implementation of `polyphony.llmgateway.v1.CompletionService`.
pub struct GrpcCompletionService {
    /// The domain use case this service delegates to. Shared with the REST router so
    /// REST and gRPC never construct two independent `CompletionService` instances.
    pub use_case: Arc<dyn CompletionUseCase>,
}

impl GrpcCompletionService {
    /// Creates a new `GrpcCompletionService` wrapping the given use case.
    ///
    /// # Arguments
    /// * `use_case` — Shared domain service implementing `CompletionUseCase`.
    ///
    /// # Returns
    /// A `GrpcCompletionService` ready to be registered with `CompletionServiceServer`.
    ///
    /// # Errors
    /// Never fails — construction is infallible.
    pub fn new(use_case: Arc<dyn CompletionUseCase>) -> Self {
        Self { use_case }
    }
}

/// Converts a proto `CompletionRequest` into the domain equivalent.
///
/// # Errors
/// Returns `tonic::Status::invalid_argument` if any message carries an unrecognized
/// `ChatRole`.
fn proto_request_to_domain(req: CompletionRequest) -> Result<domain::CompletionRequest, Status> {
    let messages = req
        .messages
        .into_iter()
        .map(|m| {
            let role = proto_role_to_domain(m.role).map_err(domain_error_to_status)?;
            Ok(domain::ChatMessage {
                role,
                content: m.content.into(),
            })
        })
        .collect::<Result<Vec<_>, Status>>()?;

    Ok(domain::CompletionRequest {
        model: req.model,
        messages,
        temperature: req.temperature,
        max_tokens: req.max_tokens,
    })
}

/// Converts a domain `CompletionResponse` into the proto equivalent.
fn domain_response_to_proto(resp: domain::CompletionResponse) -> CompletionResponse {
    CompletionResponse {
        id: resp.id,
        model: resp.model,
        choices: resp
            .choices
            .into_iter()
            .map(|c| Choice {
                index: c.index,
                message: Some(ChatMessage {
                    role: domain_role_to_proto(&c.message.role) as i32,
                    content: c.message.content.as_text(),
                }),
                finish_reason: c.finish_reason,
            })
            .collect(),
        usage: Some(Usage {
            prompt_tokens: resp.usage.prompt_tokens,
            completion_tokens: resp.usage.completion_tokens,
            total_tokens: resp.usage.total_tokens,
        }),
    }
}

#[tonic::async_trait]
impl CompletionService for GrpcCompletionService {
    /// Executes a single (non-streaming) chat completion.
    ///
    /// # Errors
    /// Returns a `tonic::Status` mapped from the underlying `DomainError` via
    /// `domain_error_to_status` (e.g. `not_found` for an unknown model).
    async fn complete(
        &self,
        request: Request<CompletionRequest>,
    ) -> Result<Response<CompletionResponse>, Status> {
        let domain_req = proto_request_to_domain(request.into_inner())?;

        let resp = self
            .use_case
            .complete(domain_req)
            .await
            .map_err(domain_error_to_status)?;

        Ok(Response::new(domain_response_to_proto(resp)))
    }
}
