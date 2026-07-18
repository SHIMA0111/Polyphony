pub mod anthropic;
pub mod env_key;
pub mod gemini;
pub mod http_retry;
pub mod openai;

use crate::domain::error::DomainError;
use crate::domain::model::{ContentPart, MessageContent};

/// Extracts the plain text of a system message's content, rejecting any non-text
/// part.
///
/// Shared by the Anthropic and Gemini adapters' `to_*_request` system-message
/// branches (both providers hoist `Role::System` messages out of the regular
/// message list into a top-level `system`/`systemInstruction` field). Unlike
/// `MessageContent::as_text()`, which silently drops non-`Text` `ContentPart`s for
/// ordinary messages, a system message must not silently drop an image part: doing
/// so would let a caller believe an image was included in the system instruction
/// when it was actually discarded.
///
/// # Arguments
/// * `content` — A system message's content to extract text from.
///
/// # Returns
/// The concatenated text of `content` (unchanged for `MessageContent::Text`; the
/// joined text of every part for `MessageContent::Parts`).
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `content` is `MessageContent::Parts`
/// and contains any part other than `ContentPart::Text`.
pub(crate) fn system_message_text(content: &MessageContent) -> Result<String, DomainError> {
    match content {
        MessageContent::Text(s) => Ok(s.clone()),
        MessageContent::Parts(parts) => {
            if parts.iter().any(|p| !matches!(p, ContentPart::Text(_))) {
                return Err(DomainError::InvalidRequest(
                    "system message content must not contain non-text parts (e.g. images)"
                        .to_string(),
                ));
            }
            Ok(parts
                .iter()
                .filter_map(|p| match p {
                    ContentPart::Text(s) => Some(s.as_str()),
                    _ => None,
                })
                .collect::<Vec<_>>()
                .join(""))
        }
    }
}
