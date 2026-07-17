//! Character-based heuristic token estimator.
//!
//! This module provides an approximate token count for a list of chat messages
//! without depending on any model-specific tokenizer library (e.g. `tiktoken`).
//! It exists so the web UI can show a live token estimate while composing a message
//! (Step 38) and so server-side context building can check a rough count against a
//! limit (Step 50) without a network round trip to a real tokenizer.
//!
//! Swapping this heuristic for a real, model-specific tokenizer is a possible future
//! enhancement, but is explicitly out of scope for this module — callers should treat
//! `estimate_tokens`'s result as approximate, not billing-grade.

use crate::domain::model::ChatMessage;

/// Approximate number of characters per token for English text.
///
/// This is the commonly cited "~4 characters per token" rule of thumb for
/// English-language text with common tokenizers (e.g. GPT/BPE-style encoders). Other
/// languages (e.g. CJK scripts) tokenize at a different ratio; this heuristic does not
/// attempt to detect language and adjust for it.
const CHARS_PER_TOKEN: f64 = 4.0;

/// Fixed per-message overhead, in tokens, added for each message to account for role
/// and metadata framing that real chat-completion tokenizers add around each message
/// (e.g. OpenAI's `<|start|>role<|message|>...<|end|>` framing tokens).
const PER_MESSAGE_OVERHEAD_TOKENS: u32 = 4;

/// Fixed one-time overhead, in tokens, added once per request to account for the
/// "reply priming" tokens most chat-completion APIs append after the message list to
/// prompt the model to begin its reply.
const REPLY_PRIMING_OVERHEAD_TOKENS: u32 = 3;

/// Estimates the total token count for a list of chat messages.
///
/// # Arguments
/// * `messages` — The messages to estimate. An empty slice is valid input (e.g. an
///   empty draft) and yields just the fixed reply-priming overhead.
///
/// # Returns
/// An approximate token count: for each message, `ceil(char_count / 4.0)` (using
/// `.chars().count()`, not byte length, so multi-byte UTF-8 characters are not
/// over-counted) plus a fixed per-message framing overhead, summed across all
/// messages, plus a single fixed reply-priming overhead.
///
/// This is a heuristic, not an exact tokenizer count — see the module docs for
/// rationale and limitations. It never fails: unlike `CompletionUseCase::complete`,
/// estimation has no model-lookup step and no `ModelNotFound` error path.
pub fn estimate_tokens(messages: &[ChatMessage]) -> u32 {
    let content_tokens: u32 = messages
        .iter()
        .map(|m| {
            let char_count = m.content.as_text().chars().count();
            let text_tokens = (char_count as f64 / CHARS_PER_TOKEN).ceil() as u32;
            text_tokens + PER_MESSAGE_OVERHEAD_TOKENS
        })
        .sum();

    content_tokens + REPLY_PRIMING_OVERHEAD_TOKENS
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::model::Role;

    fn message(role: Role, content: &str) -> ChatMessage {
        ChatMessage {
            role,
            content: content.to_string().into(),
        }
    }

    #[test]
    fn test_empty_message_list_returns_reply_priming_overhead_only() {
        assert_eq!(estimate_tokens(&[]), REPLY_PRIMING_OVERHEAD_TOKENS);
    }

    #[test]
    fn test_single_short_message() {
        // "hi" is 2 chars -> ceil(2/4) = 1 text token, + 4 per-message overhead, + 3
        // reply-priming overhead = 8.
        let messages = [message(Role::User, "hi")];
        assert_eq!(
            estimate_tokens(&messages),
            1 + PER_MESSAGE_OVERHEAD_TOKENS + REPLY_PRIMING_OVERHEAD_TOKENS
        );
    }

    #[test]
    fn test_multiple_messages_sums_each_messages_contribution() {
        let messages = [
            message(Role::System, "be concise"), // 10 chars -> ceil(10/4) = 3
            message(Role::User, "hello world"),  // 11 chars -> ceil(11/4) = 3
        ];

        let expected = (3 + PER_MESSAGE_OVERHEAD_TOKENS)
            + (3 + PER_MESSAGE_OVERHEAD_TOKENS)
            + REPLY_PRIMING_OVERHEAD_TOKENS;
        assert_eq!(estimate_tokens(&messages), expected);
    }

    #[test]
    fn test_non_ascii_multi_byte_characters_counted_as_chars_not_bytes() {
        // "こんにちは" is 5 Unicode scalar values, but each is 3 bytes in UTF-8 (15
        // bytes total). Using `.chars().count()` must yield 5, not 15.
        let messages = [message(Role::User, "こんにちは")];

        let text_tokens = (5.0_f64 / CHARS_PER_TOKEN).ceil() as u32; // ceil(5/4) = 2
        let expected = text_tokens + PER_MESSAGE_OVERHEAD_TOKENS + REPLY_PRIMING_OVERHEAD_TOKENS;
        assert_eq!(estimate_tokens(&messages), expected);
    }
}
