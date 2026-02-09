use std::future::Future;
use std::pin::Pin;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionRequest, CompletionResponse, ModelInfo};
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
    fn find_provider(&self, model: &str) -> Option<&dyn LLMProvider> {
        self.providers
            .iter()
            .find(|p| p.models().iter().any(|m| m.id == model))
            .map(|p| p.as_ref())
    }
}

impl CompletionUseCase for CompletionService {
    fn complete(
        &self,
        req: CompletionRequest,
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>> {
        Box::pin(async move {
            if req.messages.is_empty() {
                return Err(DomainError::InvalidRequest(
                    "messages must not be empty".to_string(),
                ));
            }

            let provider = self
                .find_provider(&req.model)
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

    fn list_models(&self) -> Vec<ModelInfo> {
        self.providers.iter().flat_map(|p| p.models()).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::{ChatMessage, Choice, Role, Usage};

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
        ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>>
        {
            let model = req.model.clone();
            Box::pin(async move {
                Ok(CompletionResponse {
                    id: "mock-id".to_string(),
                    model,
                    choices: vec![Choice {
                        index: 0,
                        message: ChatMessage {
                            role: Role::Assistant,
                            content: "mock response".to_string(),
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

        fn models(&self) -> Vec<ModelInfo> {
            self.model_ids
                .iter()
                .map(|id| ModelInfo {
                    id: id.clone(),
                    provider: self.name.clone(),
                    owned_by: self.name.clone(),
                })
                .collect()
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
                content: "hello".to_string(),
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
        let service = CompletionService::new(vec![Box::new(MockProvider::new(
            "openai",
            vec!["gpt-5.2"],
        ))]);

        let result = service.complete(make_request("nonexistent")).await;
        assert!(matches!(result, Err(DomainError::ModelNotFound(_))));
    }

    #[tokio::test]
    async fn test_empty_messages_rejected() {
        let service = CompletionService::new(vec![Box::new(MockProvider::new(
            "openai",
            vec!["gpt-5.2"],
        ))]);

        let req = CompletionRequest {
            model: "gpt-5.2".to_string(),
            messages: vec![],
            temperature: None,
            max_tokens: None,
        };
        let result = service.complete(req).await;
        assert!(matches!(result, Err(DomainError::InvalidRequest(_))));
    }

    #[test]
    fn test_list_models_aggregates_all_providers() {
        let service = CompletionService::new(vec![
            Box::new(MockProvider::new("openai", vec!["gpt-5.2", "gpt-5-mini"])),
            Box::new(MockProvider::new("anthropic", vec!["claude-opus-4-6"])),
        ]);

        let models = service.list_models();
        assert_eq!(models.len(), 3);
    }
}
