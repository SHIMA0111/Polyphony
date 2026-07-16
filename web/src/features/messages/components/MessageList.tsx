"use client"

import { useCallback, useRef, useSyncExternalStore } from "react"
import { Box, Button, Spinner } from "@chakra-ui/react"
import { ArrowDown } from "lucide-react"
import type { Message } from "@/features/messages/types"
import { useNearBottomScroll } from "@/features/messages/hooks/use-near-bottom-scroll"
import { useLoadOlderOnScroll } from "@/features/messages/hooks/use-load-older-on-scroll"
import { groupMessagesForDisplay } from "@/features/messages/utils/group-messages"
import { DaySeparator } from "./DaySeparator"
import { MessageGroup } from "./MessageGroup"

const noopSubscribe = () => () => {}

/**
 * `true` once hydrated on the client, `false` during SSR and the browser's
 * pre-hydration first paint — `useSyncExternalStore`'s `getServerSnapshot`
 * makes the first client render intentionally match the server's, then
 * React re-renders with the real client snapshot right after hydration
 * completes. This is the React-blessed alternative to the "set state in a
 * mount effect" pattern for values that must differ between the server
 * render and the eventual client render without causing a hydration
 * mismatch on the first paint.
 */
function useMounted(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false,
  )
}

interface MessageListProps {
  messages: Message[]
  onRegenerate: (messageId: string) => void
  isRegenerating: string | null
  /** Re-sends a failed human message's original content. */
  onRetry: (messageId: string, content: string) => void
  /** Whether an older page of history is available via `fetchNextPage`. */
  hasNextPage: boolean
  /** Whether the next (older) page is currently being fetched. */
  isFetchingNextPage: boolean
  /** Fetches the next older page of message history. */
  fetchNextPage: () => void
  /** Number of currently loaded pages, used to anchor scroll position across a load. */
  pageCount: number
}

export function MessageList({
  messages,
  onRegenerate,
  isRegenerating,
  onRetry,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  pageCount,
}: MessageListProps) {
  const { containerRef, hasNewMessages, scrollToBottom } =
    useNearBottomScroll(messages)

  // `useLoadOlderOnScroll` needs to observe the very same scrollable element
  // `useNearBottomScroll` already refs, but without changing that stable
  // Step 18 hook's signature: this second ref is attached to the same DOM
  // node via the `setContainerNode` callback ref below, so both hooks each
  // keep their own simple, independently-memoizable ref.
  const loadOlderRef = useRef<HTMLDivElement>(null)
  useLoadOlderOnScroll({
    containerRef: loadOlderRef,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    pageCount,
  })

  const setContainerNode = useCallback(
    (node: HTMLDivElement | null) => {
      containerRef.current = node
      loadOlderRef.current = node
    },
    [containerRef],
  )

  // `groupMessagesForDisplay`'s "Today"/"Yesterday" day-separator labels
  // depend on wall-clock time and the runtime's local timezone/locale, both
  // of which can differ between the Next.js server render and the browser
  // — rendering them immediately on the client would produce a React
  // hydration mismatch. Passing `null` for `now` until mounted forces the
  // exact same deterministic (UTC, fixed-locale) label on the server render
  // and the browser's pre-hydration first paint; the real `new Date()` only
  // takes over after mount, as a normal (harmless) post-hydration update.
  const mounted = useMounted()
  const items = groupMessagesForDisplay(messages, mounted ? new Date() : null)

  return (
    <Box position="relative" flex={1} minH={0}>
      <Box
        ref={setContainerNode}
        h="full"
        overflowY="auto"
        // Stable hook for `web/e2e/streaming.spec.ts` (Step 54) to attach a
        // `MutationObserver` to *before* triggering a send: the container
        // itself is mounted from the very first render, unlike any
        // individual `MessageBubble`, whose DOM node/key changes when an
        // optimistic entry is swapped for its real, server-assigned id.
        data-testid="message-list"
      >
        <Box maxW="4xl" mx="auto" px={4} py={6} spaceY={6}>
          {isFetchingNextPage && (
            <Box display="flex" justifyContent="center" py={2}>
              <Spinner size="sm" colorPalette="blue" />
            </Box>
          )}
          {items.map((item) =>
            item.kind === "day" ? (
              <DaySeparator key={`day-${item.iso}`} label={item.label} />
            ) : (
              <MessageGroup
                key={`group-${item.messages[0].id}`}
                type={item.type}
                messages={item.messages}
                onRegenerate={onRegenerate}
                isRegenerating={isRegenerating}
                onRetry={onRetry}
              />
            ),
          )}
        </Box>
      </Box>

      {hasNewMessages && (
        <Box
          position="absolute"
          bottom={4}
          left="50%"
          transform="translateX(-50%)"
        >
          <Button
            size="sm"
            rounded="full"
            colorPalette="blue"
            shadow="md"
            onClick={scrollToBottom}
          >
            <ArrowDown size={14} />
            New messages
          </Button>
        </Box>
      )}
    </Box>
  )
}
