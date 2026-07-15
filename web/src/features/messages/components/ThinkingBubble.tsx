"use client"

import { Box, Flex } from "@chakra-ui/react"

/** Stagger, in seconds, between each dot's animation start. */
const DOT_DELAYS_S = [0, 0.16, 0.32]

/**
 * Pre-response placeholder shown in place of an AI message's body while
 * `status === "sending"`: three animated dots inside the same bubble chrome
 * `MessageBubble` already uses for AI messages, so nothing jumps in layout
 * once the real content arrives.
 *
 * This is explicitly the round-trip placeholder this step needs, not a
 * token-by-token streaming renderer — that is a later step and will
 * replace/extend this component.
 *
 * The bounce animation is a plain Chakra `css`-prop `@keyframes` (per
 * `.claude/rules/chakra-ui.md`), so no extra animation dependency
 * (e.g. `framer-motion`) is needed for three dots.
 */
export function ThinkingBubble() {
  return (
    <Flex align="center" gap="1.5" py="1" aria-label="AI is thinking">
      {DOT_DELAYS_S.map((delay) => (
        <Box
          key={delay}
          w="6px"
          h="6px"
          rounded="full"
          bg="fg.muted"
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
