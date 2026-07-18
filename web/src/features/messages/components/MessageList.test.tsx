import { describe, expect, it, vi } from "vitest"
import type { Message } from "@/features/messages/types"
import { render, screen } from "@/test/render"
import { MessageList } from "./MessageList"

/**
 * Pure-component test for `MessageList` — props in, DOM/callback out, no
 * network involved. Covers a human message, a completed AI message, and a
 * `status: "failed"` AI message, asserting rendered content/labels and that
 * clicking the retry/regenerate control invokes `onRegenerate` with the
 * correct message id.
 */

const messages: Message[] = [
  {
    id: "message-human",
    room_id: "room-1",
    sender_id: "user-1",
    content: "Hello, AI!",
    type: "human",
    status: "completed",
    sequence: 1,
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "message-ai-completed",
    room_id: "room-1",
    sender_id: null,
    content: "Hello! How can I help you today?",
    type: "ai",
    status: "completed",
    sequence: 2,
    in_response_to_message_id: "message-human",
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T00:00:01Z",
    updated_at: "2026-01-01T00:00:01Z",
  },
  {
    id: "message-ai-failed",
    room_id: "room-1",
    sender_id: null,
    content: "",
    type: "ai",
    status: "failed",
    sequence: 3,
    in_response_to_message_id: "message-human",
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T00:00:02Z",
    updated_at: "2026-01-01T00:00:02Z",
  },
]

/** Shared no-op props for the pagination/retry plumbing this suite doesn't exercise. */
const noopPaginationProps = {
  onRetry: vi.fn(),
  hasNextPage: false,
  isFetchingNextPage: false,
  fetchNextPage: vi.fn(),
  pageCount: 1,
}

describe("MessageList", () => {
  it("renders human and AI message content", () => {
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.getByText("Hello, AI!")).toBeInTheDocument()
    expect(
      screen.getByText("Hello! How can I help you today?"),
    ).toBeInTheDocument()
    expect(screen.getByText("AI response failed")).toBeInTheDocument()
  })

  it("calls onRegenerate with the failed message's id when Retry is clicked", async () => {
    const onRegenerate = vi.fn()
    render(
      <MessageList
        messages={messages}
        onRegenerate={onRegenerate}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    const retryButton = screen.getByRole("button", { name: /retry/i })
    retryButton.click()

    expect(onRegenerate).toHaveBeenCalledWith("message-ai-failed")
  })

  it("calls onRegenerate with the completed message's id when Regenerate is clicked", async () => {
    const onRegenerate = vi.fn()
    render(
      <MessageList
        messages={messages}
        onRegenerate={onRegenerate}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    const regenerateButton = screen.getByRole("button", { name: /^regenerate$/i })
    regenerateButton.click()

    expect(onRegenerate).toHaveBeenCalledWith("message-ai-completed")
  })

  it("calls onRetry with the failed human message's id and content when Retry is clicked", () => {
    const onRetry = vi.fn()
    const failedHumanMessages: Message[] = [
      {
        id: "message-human-failed",
        room_id: "room-1",
        sender_id: "user-1",
        content: "This one didn't make it",
        type: "human",
        status: "failed",
        sequence: 1,
        in_response_to_message_id: null,
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility: "public",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]

    render(
      <MessageList
        messages={failedHumanMessages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
        onRetry={onRetry}
      />,
    )

    expect(screen.getByText("Message failed to send")).toBeInTheDocument()
    screen.getByRole("button", { name: /retry/i }).click()

    expect(onRetry).toHaveBeenCalledWith(
      "message-human-failed",
      "This one didn't make it",
    )
  })

  it("shows a ThinkingBubble in place of body content for a sending AI message", () => {
    const sendingAiMessages: Message[] = [
      {
        id: "message-ai-sending",
        room_id: "room-1",
        sender_id: null,
        content: "",
        type: "ai",
        status: "sending",
        sequence: 1,
        in_response_to_message_id: "message-human",
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility: "public",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]

    render(
      <MessageList
        messages={sendingAiMessages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.getByLabelText("AI is thinking")).toBeInTheDocument()
  })

  it("calls fetchNextPage when the first page doesn't fill the scroll container and more history is available", () => {
    // jsdom never lays out real dimensions, so scrollHeight/clientHeight are
    // both 0 by default -- i.e. "doesn't fill the container" is always true
    // here, exercising the same branch a genuinely short page would hit in a
    // real browser.
    const fetchNextPage = vi.fn()
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
        hasNextPage={true}
        fetchNextPage={fetchNextPage}
      />,
    )

    expect(fetchNextPage).toHaveBeenCalled()
  })

  it("does not call fetchNextPage when there is no next page, even if the container doesn't fill", () => {
    const fetchNextPage = vi.fn()
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
        hasNextPage={false}
        fetchNextPage={fetchNextPage}
      />,
    )

    expect(fetchNextPage).not.toHaveBeenCalled()
  })

  // --- Step 50: context summarization badge ---

  it("shows the 'Summarized history' badge on an AI message with used_context_summary: true", () => {
    const summarizedMessages: Message[] = [
      {
        id: "message-ai-summarized",
        room_id: "room-1",
        sender_id: null,
        content: "Here's the answer, considering our earlier discussion.",
        type: "ai",
        status: "completed",
        sequence: 1,
        in_response_to_message_id: "message-human",
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: true,
        visibility: "public",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]

    render(
      <MessageList
        messages={summarizedMessages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.getByText("Summarized history")).toBeInTheDocument()
  })

  it("does not show the 'Summarized history' badge when used_context_summary is false", () => {
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.queryByText("Summarized history")).not.toBeInTheDocument()
  })

  it("does not show the 'Summarized history' badge on a human message even if used_context_summary were true", () => {
    const humanOnly: Message[] = [
      {
        id: "message-human-2",
        room_id: "room-1",
        sender_id: "user-1",
        content: "Just a normal question",
        type: "human",
        status: "completed",
        sequence: 1,
        in_response_to_message_id: null,
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: true,
        visibility: "public",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]

    render(
      <MessageList
        messages={humanOnly}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.queryByText("Summarized history")).not.toBeInTheDocument()
  })

  // --- Step 47: private AI mode badge ---

  it("shows the 'Private' badge on a message with visibility: 'private'", () => {
    const privateMessages: Message[] = [
      {
        id: "message-private-human",
        room_id: "room-1",
        sender_id: "user-1",
        content: "What's my salary review outcome?",
        type: "human",
        status: "completed",
        sequence: 1,
        in_response_to_message_id: null,
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility: "private",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      {
        id: "message-private-ai",
        room_id: "room-1",
        sender_id: null,
        content: "I can't know that.",
        type: "ai",
        status: "completed",
        sequence: 2,
        in_response_to_message_id: "message-private-human",
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility: "private",
        created_at: "2026-01-01T00:00:01Z",
        updated_at: "2026-01-01T00:00:01Z",
      },
    ]

    render(
      <MessageList
        messages={privateMessages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.getAllByText("Private")).toHaveLength(2)
  })

  it("does not show the 'Private' badge for visibility: 'public' messages", () => {
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
        {...noopPaginationProps}
      />,
    )

    expect(screen.queryByText("Private")).not.toBeInTheDocument()
  })
})
