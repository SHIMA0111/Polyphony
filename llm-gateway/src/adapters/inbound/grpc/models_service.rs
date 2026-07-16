//! gRPC `ModelsService` implementation.
//!
//! Delegates to the same `Arc<dyn CompletionUseCase>` the REST `GET /models` handler
//! uses (`adapters::inbound::rest::handlers::list_models`), so REST and gRPC always
//! surface identical model metadata.

use std::sync::Arc;

use tonic::{Request, Response, Status};

use crate::domain::model::ModelInfo as DomainModelInfo;
use crate::ports::inbound::completion::CompletionUseCase;

use super::pb::models_service_server::ModelsService;
use super::pb::{ListModelsRequest, ListModelsResponse, ModelInfo, ModelPricing};

/// Tonic server implementation of `polyphony.llmgateway.v1.ModelsService`.
pub struct GrpcModelsService {
    /// The domain use case this service delegates to. Shared with the REST router so
    /// REST and gRPC never construct two independent `CompletionService` instances.
    pub use_case: Arc<dyn CompletionUseCase>,
}

impl GrpcModelsService {
    /// Creates a new `GrpcModelsService` wrapping the given use case.
    ///
    /// # Arguments
    /// * `use_case` — Shared domain service implementing `CompletionUseCase`.
    pub fn new(use_case: Arc<dyn CompletionUseCase>) -> Self {
        Self { use_case }
    }
}

/// Converts a domain `ModelInfo` into the proto equivalent, carrying over the
/// optional `context_window`/`pricing`/`supports_image_input` metadata as-is.
fn domain_model_to_proto(m: DomainModelInfo) -> ModelInfo {
    ModelInfo {
        id: m.id,
        name: m.name,
        provider: m.provider,
        owned_by: m.owned_by,
        context_window: m.context_window,
        pricing: m.pricing.map(|p| ModelPricing {
            input_price_per_million_tokens: p.input_price_per_million_tokens,
            output_price_per_million_tokens: p.output_price_per_million_tokens,
            currency: p.currency,
        }),
        supports_image_input: m.supports_image_input,
    }
}

#[tonic::async_trait]
impl ModelsService for GrpcModelsService {
    /// Returns all available models across all configured providers.
    ///
    /// # Errors
    /// Never returns an error today — `CompletionUseCase::list_models` is infallible.
    async fn list_models(
        &self,
        _request: Request<ListModelsRequest>,
    ) -> Result<Response<ListModelsResponse>, Status> {
        let models = self
            .use_case
            .list_models()
            .await
            .into_iter()
            .map(domain_model_to_proto)
            .collect();

        Ok(Response::new(ListModelsResponse { models }))
    }
}
