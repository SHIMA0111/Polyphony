pub mod anthropic;
pub mod env_key;
pub mod gemini;
pub mod http_retry;
pub mod openai;

use crate::domain::error::DomainError;
use crate::domain::model::{ContentPart, MessageContent};

/// Extracts the plain text of a system message's content, rejecting image parts.
///
/// Both the Anthropic and Gemini adapters hoist `Role::System` messages out of the
/// per-turn message list into a dedicated top-level field (Anthropic's `system` is a
/// flat string; Gemini's `systemInstruction` is built from text-only parts by this
/// adapter). Neither of those fields can carry an image, so unlike the user/assistant
/// content path — which maps `ContentPart::ImageUrl`/`ImageBase64` to real image
/// blocks — a system message containing an image part has no representable target and
/// must be rejected here rather than silently dropped by
/// [`MessageContent::as_text`][text], which ignores non-text parts.
///
/// [text]: crate::domain::model::MessageContent::as_text
///
/// # Arguments
/// * `content` — The system message's `MessageContent`.
///
/// # Returns
/// The concatenated text: `content` as-is for `MessageContent::Text`, or the joined
/// text of every part for `MessageContent::Parts`.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `content` is `MessageContent::Parts` and
/// contains any part other than `ContentPart::Text`.
pub(crate) fn system_message_text(content: &MessageContent) -> Result<String, DomainError> {
    match content {
        MessageContent::Text(s) => Ok(s.clone()),
        MessageContent::Parts(parts) => {
            if parts.iter().any(|p| !matches!(p, ContentPart::Text(_))) {
                return Err(DomainError::InvalidRequest(
                    "system message content must not contain image parts".to_string(),
                ));
            }
            Ok(content.as_text())
        }
    }
}
