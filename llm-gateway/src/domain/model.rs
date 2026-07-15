use crate::domain::error::DomainError;

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

impl Role {
    /// Parses a wire-format role string into a domain `Role`.
    ///
    /// This is the single canonical wire→domain mapping used by every inbound/outbound
    /// adapter that needs to interpret a role string, so aliasing rules (e.g. OpenAI's
    /// `"developer"` for `System`) stay consistent everywhere.
    ///
    /// # Arguments
    /// * `s` — Role string as received from a client or provider (e.g. `"system"`, `"developer"`).
    ///
    /// # Returns
    /// The corresponding `Role`: `"system"`/`"developer"` → `System`, `"user"` → `User`,
    /// `"assistant"` → `Assistant`, `"tool"`/`"function"` (legacy alias) → `Tool`.
    ///
    /// # Errors
    /// Returns `DomainError::InvalidRequest` if `s` does not match any known role string.
    pub fn parse(s: &str) -> Result<Role, DomainError> {
        match s {
            "system" | "developer" => Ok(Role::System),
            "user" => Ok(Role::User),
            "assistant" => Ok(Role::Assistant),
            "tool" | "function" => Ok(Role::Tool),
            other => Err(DomainError::InvalidRequest(format!(
                "unknown role: {other}"
            ))),
        }
    }

    /// Returns the canonical REST-wire string for this role.
    ///
    /// # Returns
    /// One of `"system"`, `"user"`, `"assistant"`, `"tool"`.
    pub fn as_str(&self) -> &'static str {
        match self {
            Role::System => "system",
            Role::User => "user",
            Role::Assistant => "assistant",
            Role::Tool => "tool",
        }
    }
}

/// A single part of a multimodal message's content.
///
/// `ImageUrl`/`ImageBase64` are not yet consumed anywhere (Vision support lands in a
/// later phase); they exist now so `MessageContent` can represent them without another
/// domain rewrite when that phase arrives.
// Only constructed in tests today — allowed dead code until a later Vision step
// starts producing these variants from real request/response parsing.
#[allow(dead_code)]
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ContentPart {
    /// A plain text segment.
    Text(String),
    /// A reference to an image by URL.
    ImageUrl(String),
    /// An inline base64-encoded image.
    ImageBase64 {
        /// MIME type of the image (e.g. `"image/png"`).
        media_type: String,
        /// Base64-encoded image bytes.
        data: String,
    },
}

/// Content of a chat message.
///
/// Most messages today are plain text (`Text`); `Parts` is provided so future Vision
/// input can be represented without changing `ChatMessage`'s shape again.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MessageContent {
    /// Plain text content.
    Text(String),
    /// A sequence of content parts (text mixed with images).
    // Only constructed in tests today — allowed dead code until a later Vision step
    // starts producing this variant from real request parsing.
    #[allow(dead_code)]
    Parts(Vec<ContentPart>),
}

impl MessageContent {
    /// Downgrades this content to plain text.
    ///
    /// This is the single place that flattens multimodal content for wire formats
    /// that don't support it yet (currently, the REST boundary). For `Text`, returns
    /// the string as-is. For `Parts`, concatenates only the `ContentPart::Text` entries;
    /// image parts contribute nothing (full Vision handling is a later step).
    ///
    /// # Returns
    /// The plain-text representation of this content.
    pub fn as_text(&self) -> String {
        match self {
            MessageContent::Text(s) => s.clone(),
            MessageContent::Parts(parts) => parts
                .iter()
                .filter_map(|p| match p {
                    ContentPart::Text(s) => Some(s.as_str()),
                    _ => None,
                })
                .collect::<Vec<_>>()
                .join(""),
        }
    }
}

impl From<String> for MessageContent {
    fn from(s: String) -> Self {
        MessageContent::Text(s)
    }
}

/// A chat message consisting of a role and content.
#[derive(Debug, Clone)]
pub struct ChatMessage {
    pub role: Role,
    pub content: MessageContent,
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
    pub name: String,
    pub provider: String,
    pub owned_by: String,
}

/// A single chunk of a streamed completion response.
///
/// Emitted incrementally by `LLMProvider::stream`/`CompletionUseCase::stream` as a
/// provider produces its response. Not yet implemented by any adapter in this step;
/// this type exists so the streaming port signatures can land ahead of a real
/// SSE-parsing implementation.
// Not yet constructed anywhere — no provider adapter streams real chunks yet.
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct CompletionChunk {
    pub id: String,
    pub model: String,
    /// Incremental text content produced since the previous chunk, if any.
    pub delta: Option<String>,
    /// Set on the final chunk to indicate why generation stopped.
    pub finish_reason: Option<String>,
    /// Token usage, typically only populated on the final chunk.
    pub usage: Option<Usage>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_role_parse_accepts_known_strings() {
        assert_eq!(Role::parse("system").unwrap(), Role::System);
        assert_eq!(Role::parse("developer").unwrap(), Role::System);
        assert_eq!(Role::parse("user").unwrap(), Role::User);
        assert_eq!(Role::parse("assistant").unwrap(), Role::Assistant);
        assert_eq!(Role::parse("tool").unwrap(), Role::Tool);
        assert_eq!(Role::parse("function").unwrap(), Role::Tool);
    }

    #[test]
    fn test_role_parse_rejects_unknown_string() {
        assert!(matches!(
            Role::parse("bogus"),
            Err(DomainError::InvalidRequest(_))
        ));
    }

    /// Regression test for the bug this step fixes: the REST inbound parser used to
    /// reject `"developer"` while the OpenAI adapter's parser accepted it. Both now go
    /// through this single `Role::parse`.
    #[test]
    fn test_role_parse_developer_is_system() {
        assert_eq!(Role::parse("developer").unwrap(), Role::System);
    }

    #[test]
    fn test_role_as_str_round_trips_canonical_roles() {
        assert_eq!(Role::System.as_str(), "system");
        assert_eq!(Role::User.as_str(), "user");
        assert_eq!(Role::Assistant.as_str(), "assistant");
        assert_eq!(Role::Tool.as_str(), "tool");
    }

    #[test]
    fn test_message_content_as_text_plain_text() {
        let content = MessageContent::Text("hello".to_string());
        assert_eq!(content.as_text(), "hello");
    }

    #[test]
    fn test_message_content_as_text_parts_all_text() {
        let content = MessageContent::Parts(vec![
            ContentPart::Text("hello ".to_string()),
            ContentPart::Text("world".to_string()),
        ]);
        assert_eq!(content.as_text(), "hello world");
    }

    #[test]
    fn test_message_content_as_text_parts_skips_images() {
        let content = MessageContent::Parts(vec![
            ContentPart::Text("look: ".to_string()),
            ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
            ContentPart::ImageBase64 {
                media_type: "image/png".to_string(),
                data: "abc123".to_string(),
            },
            ContentPart::Text(" cute!".to_string()),
        ]);
        assert_eq!(content.as_text(), "look:  cute!");
    }
}
