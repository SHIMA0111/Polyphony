use serde::{Deserialize, Serialize};

use crate::domain::error::DomainError;
use crate::domain::model::{
    ChatMessage, CompletionRequest, ContentPart, MessageContent, Role, TokenEstimateRequest,
};

/// Completion request DTO for the REST API.
#[derive(Debug, Deserialize)]
pub struct CompletionRequestDto {
    pub model: String,
    pub messages: Vec<MessageDto>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

/// Message DTO for the REST API.
///
/// Roles are received as strings and converted to the domain `Role`.
/// The REST API accepts OpenAI-compatible strings ("system", "user", "assistant", "tool").
#[derive(Debug, Deserialize)]
pub struct MessageDto {
    pub role: String,
    pub content: ContentDto,
}

/// Wire representation of a message's `content` field, accepted (and emitted, for
/// responses) in either of two shapes:
/// - a plain JSON string (backward compatible with the pre-Vision REST contract),
///   mapping to `MessageContent::Text`;
/// - an array of content-part objects (mirroring OpenAI's multimodal `content` array
///   shape), mapping to `MessageContent::Parts`.
///
/// `#[serde(untagged)]` tries each variant in declaration order, so a JSON string
/// always resolves to `Text` and a JSON array always resolves to `Parts` — the two
/// shapes never overlap.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize)]
#[serde(untagged)]
pub enum ContentDto {
    /// Plain text content (the original, pre-Vision REST contract shape).
    Text(String),
    /// A sequence of content parts (text mixed with images).
    Parts(Vec<ContentPartDto>),
}

/// Wire representation of a single content part, tagged by `"type"` (mirroring
/// OpenAI's `content` array shape, since that is the closest existing convention this
/// gateway already exposes over REST).
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ContentPartDto {
    /// A plain text segment: `{"type": "text", "text": "..."}`.
    Text {
        /// The text content of this part.
        text: String,
    },
    /// A reference to an image by URL: `{"type": "image_url", "image_url": {"url": "..."}}`.
    ImageUrl {
        /// The image URL wrapper, matching OpenAI's nested `image_url.url` shape.
        image_url: ImageUrlDto,
    },
    /// An inline base64-encoded image:
    /// `{"type": "image_base64", "media_type": "...", "data": "..."}`.
    ImageBase64 {
        /// MIME type of the image (e.g. `"image/png"`).
        media_type: String,
        /// Base64-encoded image bytes.
        data: String,
    },
}

/// Nested `image_url` object of a `ContentPartDto::ImageUrl`, matching OpenAI's wire
/// shape (`{"url": "..."}`) rather than a bare string.
#[derive(Debug, Clone, PartialEq, Deserialize, Serialize)]
pub struct ImageUrlDto {
    /// The image's URL.
    pub url: String,
}

impl ContentDto {
    /// Converts this DTO into the domain `MessageContent`.
    pub fn into_domain(self) -> MessageContent {
        match self {
            ContentDto::Text(s) => MessageContent::Text(s),
            ContentDto::Parts(parts) => {
                MessageContent::Parts(parts.into_iter().map(ContentPartDto::into_domain).collect())
            }
        }
    }
}

impl ContentPartDto {
    /// Converts this DTO into the domain `ContentPart`.
    pub fn into_domain(self) -> ContentPart {
        match self {
            ContentPartDto::Text { text } => ContentPart::Text(text),
            ContentPartDto::ImageUrl { image_url } => ContentPart::ImageUrl(image_url.url),
            ContentPartDto::ImageBase64 { media_type, data } => {
                ContentPart::ImageBase64 { media_type, data }
            }
        }
    }
}

impl From<MessageContent> for ContentDto {
    fn from(content: MessageContent) -> Self {
        match content {
            MessageContent::Text(s) => ContentDto::Text(s),
            MessageContent::Parts(parts) => {
                ContentDto::Parts(parts.into_iter().map(ContentPartDto::from).collect())
            }
        }
    }
}

impl From<ContentPart> for ContentPartDto {
    fn from(part: ContentPart) -> Self {
        match part {
            ContentPart::Text(text) => ContentPartDto::Text { text },
            ContentPart::ImageUrl(url) => ContentPartDto::ImageUrl {
                image_url: ImageUrlDto { url },
            },
            ContentPart::ImageBase64 { media_type, data } => {
                ContentPartDto::ImageBase64 { media_type, data }
            }
        }
    }
}

impl CompletionRequestDto {
    /// Converts this DTO into a domain model.
    ///
    /// # Errors
    /// Returns `DomainError::InvalidRequest` if an unknown role string is encountered.
    pub fn into_domain(self) -> Result<CompletionRequest, DomainError> {
        let messages = self
            .messages
            .into_iter()
            .map(|m| {
                Ok(ChatMessage {
                    role: Role::parse(&m.role)?,
                    content: m.content.into_domain(),
                })
            })
            .collect::<Result<Vec<_>, DomainError>>()?;

        Ok(CompletionRequest {
            model: self.model,
            messages,
            temperature: self.temperature,
            max_tokens: self.max_tokens,
        })
    }
}

/// Token estimation request DTO for the REST API.
///
/// Deliberately mirrors `CompletionRequestDto`'s shape (minus sampling parameters)
/// rather than reusing it directly, so this endpoint's request contract can evolve
/// independently of `/completions`'.
#[derive(Debug, Deserialize)]
pub struct TokenEstimateRequestDto {
    pub model: String,
    pub messages: Vec<MessageDto>,
}

impl TokenEstimateRequestDto {
    /// Converts this DTO into a domain model.
    ///
    /// Unlike `CompletionRequestDto::into_domain`, an empty `messages` array is **not**
    /// rejected here: a draft with no messages yet is a valid estimation input (it
    /// yields the fixed reply-priming overhead only), whereas `/completions` requires
    /// at least one message to actually generate a completion.
    ///
    /// # Errors
    /// Returns `DomainError::InvalidRequest` if an unknown role string is encountered.
    pub fn into_domain(self) -> Result<TokenEstimateRequest, DomainError> {
        let messages = self
            .messages
            .into_iter()
            .map(|m| {
                Ok(ChatMessage {
                    role: Role::parse(&m.role)?,
                    content: m.content.into_domain(),
                })
            })
            .collect::<Result<Vec<_>, DomainError>>()?;

        Ok(TokenEstimateRequest {
            model: self.model,
            messages,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Regression test: the REST-inbound parser previously rejected the `"developer"`
    /// role while the OpenAI adapter's parser accepted it. Both now share `Role::parse`,
    /// so `"developer"` is accepted here too.
    #[test]
    fn test_into_domain_accepts_developer_role() {
        let dto = CompletionRequestDto {
            model: "gpt-5.2".to_string(),
            messages: vec![MessageDto {
                role: "developer".to_string(),
                content: ContentDto::Text("be concise".to_string()),
            }],
            temperature: None,
            max_tokens: None,
        };

        let req = dto
            .into_domain()
            .expect("developer role should be accepted");
        assert_eq!(req.messages[0].role, Role::System);
    }

    /// Unlike `CompletionRequestDto`, an empty `messages` array is a valid estimation
    /// input (e.g. an empty draft), not an error.
    #[test]
    fn test_token_estimate_into_domain_accepts_empty_messages() {
        let dto = TokenEstimateRequestDto {
            model: "gpt-5.2".to_string(),
            messages: vec![],
        };

        let req = dto
            .into_domain()
            .expect("empty messages should be accepted for estimation");
        assert!(req.messages.is_empty());
    }

    #[test]
    fn test_token_estimate_into_domain_rejects_unknown_role() {
        let dto = TokenEstimateRequestDto {
            model: "gpt-5.2".to_string(),
            messages: vec![MessageDto {
                role: "bogus".to_string(),
                content: ContentDto::Text("hi".to_string()),
            }],
        };

        assert!(matches!(
            dto.into_domain(),
            Err(DomainError::InvalidRequest(_))
        ));
    }

    /// A plain JSON string `content` field deserializes to `ContentDto::Text` and maps
    /// to `MessageContent::Text`, preserving the pre-Vision REST contract shape.
    #[test]
    fn test_content_dto_deserializes_plain_string_as_text() {
        let dto: ContentDto = serde_json::from_str(r#""hello world""#).unwrap();
        assert_eq!(dto, ContentDto::Text("hello world".to_string()));
        assert_eq!(
            dto.into_domain(),
            MessageContent::Text("hello world".to_string())
        );
    }

    /// A JSON array of typed part objects deserializes to `ContentDto::Parts` and maps
    /// to `MessageContent::Parts`, covering all three part shapes (text, image URL,
    /// inline base64 image).
    #[test]
    fn test_content_dto_deserializes_parts_array() {
        let json = serde_json::json!([
            {"type": "text", "text": "look:"},
            {"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}},
            {"type": "image_base64", "media_type": "image/png", "data": "abcd"},
        ]);
        let dto: ContentDto = serde_json::from_value(json).unwrap();

        let domain = dto.into_domain();
        assert_eq!(
            domain,
            MessageContent::Parts(vec![
                ContentPart::Text("look:".to_string()),
                ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
                ContentPart::ImageBase64 {
                    media_type: "image/png".to_string(),
                    data: "abcd".to_string(),
                },
            ])
        );
    }

    /// `into_domain` round-trips a multimodal `MessageDto.content` through
    /// `CompletionRequestDto::into_domain` into a `ChatMessage` carrying
    /// `MessageContent::Parts`.
    #[test]
    fn test_into_domain_accepts_parts_content() {
        let dto = CompletionRequestDto {
            model: "gpt-5.2".to_string(),
            messages: vec![MessageDto {
                role: "user".to_string(),
                content: ContentDto::Parts(vec![ContentPartDto::Text {
                    text: "hi".to_string(),
                }]),
            }],
            temperature: None,
            max_tokens: None,
        };

        let req = dto.into_domain().expect("parts content should be accepted");
        assert!(matches!(req.messages[0].content, MessageContent::Parts(_)));
    }

    /// `ContentDto`/`MessageContent` convert back and forth without loss, for both the
    /// `Text` and `Parts` shapes.
    #[test]
    fn test_content_dto_round_trips_with_message_content() {
        let text = MessageContent::Text("hi".to_string());
        assert_eq!(ContentDto::from(text.clone()).into_domain(), text);

        let parts = MessageContent::Parts(vec![
            ContentPart::Text("look: ".to_string()),
            ContentPart::ImageUrl("https://example.com/cat.png".to_string()),
            ContentPart::ImageBase64 {
                media_type: "image/png".to_string(),
                data: "abcd".to_string(),
            },
        ]);
        assert_eq!(ContentDto::from(parts.clone()).into_domain(), parts);
    }
}
