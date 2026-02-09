/// Chat message role.
///
/// Represents the conceptual role within the application.
/// Conversion to provider-specific role strings (e.g. OpenAI's "developer", Gemini's "model")
/// is handled in the adapter layer.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Role {
    /// System instructions / context setting.
    System,
    /// User input.
    User,
    /// AI response.
    Assistant,
    /// Tool execution result (web search, function calls, etc.).
    Tool,
}

/// A chat message consisting of a role and content.
#[derive(Debug, Clone)]
pub struct ChatMessage {
    pub role: Role,
    pub content: String,
}

/// LLM completion request. Provider-agnostic domain model.
#[derive(Debug, Clone)]
pub struct CompletionRequest {
    pub model: String,
    pub messages: Vec<ChatMessage>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

/// LLM completion response. Provider-agnostic domain model.
#[derive(Debug, Clone)]
pub struct CompletionResponse {
    pub id: String,
    pub model: String,
    pub choices: Vec<Choice>,
    pub usage: Usage,
}

/// A choice within a completion response.
#[derive(Debug, Clone)]
pub struct Choice {
    pub index: u32,
    pub message: ChatMessage,
    pub finish_reason: String,
}

/// Token usage statistics.
#[derive(Debug, Clone)]
pub struct Usage {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// Information about an available model.
#[derive(Debug, Clone)]
pub struct ModelInfo {
    pub id: String,
    pub provider: String,
    pub owned_by: String,
}
