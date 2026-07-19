"use client"

import { Box, Flex } from "@chakra-ui/react"

/** Stagger, in seconds, between each dot's animation start. */
const DOT_DELAYS_S = [0, 0.16, 0.32]

/**
 * Pre-response placeholder shown in place of an AI message's body while no
 * content has arrived yet: three animated dots inside the same bubble
 * chrome `MessageBubble` already uses for AI messages, so nothing jumps in
 * layout once the real content arrives.
 *
 * Covers two of `MessageBubble`'s AI states (Step 54): the pre-round-trip
 * optimistic placeholder (`status === "sending"`) and a persisted streaming
 * placeholder that hasn't received its first `token_chunk` yet
 * (`status === "streaming"` with empty `content`). Once content starts
 * arriving, `MessageBubble` swaps to its own live-streaming text rendering
 * instead (a growing `Text` with a pulsing cursor), and finally to
 * `MarkdownContent` once the message is finalized.
 *
 * The bounce animation is a plain Chakra `css`-prop `@keyframes` (per
 * `.claude/rules/chakra-ui.md`), so no extra animation dependency
 * (e.g. `framer-motion`) is needed for three dots.
 *
 * Accessibility: `role="status"` (an implicit `aria-live="polite"` region)
 * on the container announces "AI is thinking" to assistive technology once,
 * without interrupting whatever the user is currently doing — a plain
 * `aria-label` alone is not announced unless the element is otherwise
 * focused/read, and this container is neither. The three dot `Box`es are
 * purely decorative motion with no independent meaning, so they carry
 * `aria-hidden` to keep the live region's announced content to the
 * container's own label instead of three unlabeled child nodes.
 */
export function ThinkingBubble() {
  return (
    <Flex align="center" gap="1.5" py="1" role="status" aria-label="AI is thinking">
      {DOT_DELAYS_S.map((delay) => (
        <Box
          key={delay}
          w="6px"
          h="6px"
          rounded="full"
          bg="fg.muted"
          aria-hidden="true"
          css={{
            "@keyframes thinking-bounce": {
              "0%, 80%, 100%": { opacity: 0.35, transform: "scale(0.7)" },
              "40%": { opacity: 1, transform: "scale(1)" },
            },
            animation: "thinking-bounce 1.2s ease-in-out infinite",
            animationDelay: `${delay}s`,
          }}
        />
      ))}
    </Flex>
  )
}
