use futures::future::BoxFuture;
use futures::stream::BoxStream;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionChunk, CompletionRequest, CompletionResponse, ModelInfo};
use crate::ports::inbound::completion::CompletionUseCase;
use crate::ports::outbound::provider::LLMProvider;

/// Completion domain service.
///
/// Holds multiple LLM providers and dispatches completion requests
/// to the appropriate provider based on the requested model name.
pub struct CompletionService {
    providers: Vec<Box<dyn LLMProvider>>,
}

impl CompletionService {
    /// Creates a new `CompletionService`.
    ///
    /// # Arguments
    /// * `providers` — List of available LLM providers
    pub fn new(providers: Vec<Box<dyn LLMProvider>>) -> Self {
        Self { providers }
    }

    /// Finds a provider that supports the given model ID.
    ///
    /// # Arguments
    /// * `model` — Model ID to look up.
    ///
    /// # Returns
    /// The first provider (in registration order) whose `models()` list contains
    /// `model`, or `None` if no provider offers it.
    async fn find_provider(&self, model: &str) -> Option<&dyn LLMProvider> {
        for p in &self.providers {
            if p.models().await.iter().any(|m| m.id == model) {
                return Some(p.as_ref());
            }
        }
        None
    }
}

impl CompletionUseCase for CompletionService {
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

            let provider = self
                .find_provider(&req.model)
                .await
                .ok_or_else(|| DomainError::ModelNotFound(req.model.clone()))?;

            tracing::info!(
                model = %req.model,
                provider = %provider.provider_name(),
                message_count = req.messages.len(),
                "dispatching completion request"
            );

            provider.complete(&req).await
        })
    }

    fn list_models(&self) -> BoxFuture<'_, Vec<ModelInfo>> {
        Box::pin(async move {
            let mut models = Vec::new();
            for p in &self.providers {
                models.extend(p.models().await);
            }
            models
        })
    }

    fn stream(
        &self,
        req: CompletionRequest,
    ) -> BoxFuture<'_, Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>>
    {
        Box::pin(async move {
            if req.messages.is_empty() {
                return Err(DomainError::InvalidRequest(
                    "messages must not be empty".to_string(),
                ));
            }

            let provider = self
                .find_provider(&req.model)
                .await
                .ok_or_else(|| DomainError::ModelNotFound(req.model.clone()))?;

            provider.stream(&req).await
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::{ChatMessage, Choice, Role, Usage};
    use futures::stream::BoxStream;

    /// Mock provider for testing.
    struct MockProvider {
        name: String,
        model_ids: Vec<String>,
    }

    impl MockProvider {
        fn new(name: &str, model_ids: Vec<&str>) -> Self {
            Self {
                name: name.to_string(),
                model_ids: model_ids.into_iter().map(String::from).collect(),
            }
        }
    }

    impl LLMProvider for MockProvider {
        fn complete(
            &self,
            req: &CompletionRequest,
        ) -> BoxFuture<'_, Result<CompletionResponse, DomainError>> {
            let model = req.model.clone();
            Box::pin(async move {
                Ok(CompletionResponse {
                    id: "mock-id".to_string(),
                    model,
                    choices: vec![Choice {
                        index: 0,
                        message: ChatMessage {
                            role: Role::Assistant,
                            content: "mock response".to_string().into(),
                        },
                        finish_reason: "stop".to_string(),
                    }],
                    usage: Usage {
                        prompt_tokens: 10,
                        completion_tokens: 5,
                        total_tokens: 15,
                    },
                })
            })
        }

        fn models(&self) -> BoxFuture<'_, Vec<ModelInfo>> {
            let models = self
                .model_ids
                .iter()
                .map(|id| ModelInfo {
                    id: id.clone(),
                    name: id.clone(),
                    provider: self.name.clone(),
                    owned_by: self.name.clone(),
                })
                .collect();
            Box::pin(async move { models })
        }

        fn stream(
            &self,
            _req: &CompletionRequest,
        ) -> BoxFuture<
            '_,
            Result<BoxStream<'static, Result<CompletionChunk, DomainError>>, DomainError>,
        > {
            Box::pin(async move {
                Err(DomainError::provider_error(
                    "streaming not supported by mock provider",
                ))
            })
        }

        fn provider_name(&self) -> &str {
            &self.name
        }
    }

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

    #[tokio::test]
    async fn test_routes_to_correct_provider() {
        let service = CompletionService::new(vec![
            Box::new(MockProvider::new("openai", vec!["gpt-5.2", "gpt-5-mini"])),
            Box::new(MockProvider::new("anthropic", vec!["claude-opus-4-6"])),
        ]);

        let resp = service.complete(make_request("gpt-5.2")).await.unwrap();
        assert_eq!(resp.model, "gpt-5.2");

        let resp = service
            .complete(make_request("claude-opus-4-6"))
            .await
            .unwrap();
        assert_eq!(resp.model, "claude-opus-4-6");
    }

    #[tokio::test]
    async fn test_model_not_found() {
        let service =
            CompletionService::new(vec![Box::new(MockProvider::new("openai", vec!["gpt-5.2"]))]);

        let result = service.complete(make_request("nonexistent")).await;
        assert!(matches!(result, Err(DomainError::ModelNotFound(_))));
    }

    #[tokio::test]
    async fn test_empty_messages_rejected() {
        let service =
            CompletionService::new(vec![Box::new(MockProvider::new("openai", vec!["gpt-5.2"]))]);

        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![],
            temperature: None,
            max_tokens: None,
        };
        let result = service.complete(req).await;
        assert!(matches!(result, Err(DomainError::InvalidRequest(_))));
    }

    #[tokio::test]
    async fn test_stream_empty_messages_rejected() {
        let service =
            CompletionService::new(vec![Box::new(MockProvider::new("openai", vec!["gpt-5.2"]))]);

        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![],
            temperature: None,
            max_tokens: None,
        };
        let result = service.stream(req).await;
        assert!(matches!(result, Err(DomainError::InvalidRequest(_))));
    }

    #[tokio::test]
    async fn test_list_models_aggregates_all_providers() {
        let service = CompletionService::new(vec![
            Box::new(MockProvider::new("openai", vec!["gpt-5.2", "gpt-5-mini"])),
            Box::new(MockProvider::new("anthropic", vec!["claude-opus-4-6"])),
        ]);

        let models = service.list_models().await;
        assert_eq!(models.len(), 3);
    }
}
