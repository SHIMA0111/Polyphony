//! gRPC `CompletionService` implementation.
//!
//! Delegates to the same `Arc<dyn CompletionUseCase>` the REST `POST /completions`
//! handler uses (`adapters::inbound::rest::handlers::complete`); this adapter only
//! translates between the wire (proto) and domain shapes.

use std::sync::Arc;

use tonic::{Request, Response, Status};

use crate::domain::model as domain;
use crate::ports::inbound::completion::CompletionUseCase;

use super::convert::{
    domain_content_to_proto, domain_error_to_status, domain_role_to_proto, proto_content_to_domain,
    proto_role_to_domain,
};
use super::pb::completion_service_server::CompletionService;
use super::pb::{
    ChatMessage, Choice, CompletionRequest, CompletionResponse, TokenEstimateRequest,
    TokenEstimateResponse, Usage,
};

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
/// `ChatRole` or a malformed content part (see `proto_content_to_domain`).
fn proto_request_to_domain(req: CompletionRequest) -> Result<domain::CompletionRequest, Status> {
    let messages = req
        .messages
        .into_iter()
        .map(|m| {
            let role = proto_role_to_domain(m.role).map_err(domain_error_to_status)?;
            let content = proto_content_to_domain(m.content).map_err(domain_error_to_status)?;
            Ok(domain::ChatMessage { role, content })
        })
        .collect::<Result<Vec<_>, Status>>()?;

    Ok(domain::CompletionRequest {
        model: req.model,
        messages,
        temperature: req.temperature,
        max_tokens: req.max_tokens,
    })
}

/// Converts a proto `TokenEstimateRequest` into the domain equivalent.
///
/// Mirrors `proto_request_to_domain` but without the sampling parameters
/// (`temperature`, `max_tokens`) that `TokenEstimateRequest` does not carry.
///
/// # Errors
/// Returns `tonic::Status::invalid_argument` if any message carries an unrecognized
/// `ChatRole` or a malformed content part (see `proto_content_to_domain`).
fn proto_token_estimate_to_domain(
    req: TokenEstimateRequest,
) -> Result<domain::TokenEstimateRequest, Status> {
    let messages = req
        .messages
        .into_iter()
        .map(|m| {
            let role = proto_role_to_domain(m.role).map_err(domain_error_to_status)?;
            let content = proto_content_to_domain(m.content).map_err(domain_error_to_status)?;
            Ok(domain::ChatMessage { role, content })
        })
        .collect::<Result<Vec<_>, Status>>()?;

    Ok(domain::TokenEstimateRequest {
        model: req.model,
        messages,
    })
}

/// Converts a domain `TokenEstimateResponse` into the proto equivalent.
fn domain_token_estimate_response_to_proto(
    resp: domain::TokenEstimateResponse,
) -> TokenEstimateResponse {
    TokenEstimateResponse {
        model: resp.model,
        estimated_tokens: resp.estimated_tokens,
    }
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
                    content: Some(domain_content_to_proto(c.message.content)),
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

    /// Returns an approximate token count for a list of chat messages.
    ///
    /// Delegates to the same `CompletionUseCase::estimate_tokens` heuristic the REST
    /// `POST /tokens/estimate` handler uses (`adapters::inbound::rest::handlers::estimate_tokens`).
    ///
    /// # Errors
    /// Returns `tonic::Status::invalid_argument` if any message carries an unrecognized
    /// `ChatRole` or a malformed content part (see `proto_content_to_domain`). Unlike
    /// `complete`, an unrecognized model name never causes an error (estimation does
    /// not require a known model — see `CompletionUseCase::estimate_tokens`).
    async fn estimate_tokens(
        &self,
        request: Request<TokenEstimateRequest>,
    ) -> Result<Response<TokenEstimateResponse>, Status> {
        let domain_req = proto_token_estimate_to_domain(request.into_inner())?;
        let resp = self.use_case.estimate_tokens(domain_req);
        Ok(Response::new(domain_token_estimate_response_to_proto(resp)))
    }
}
