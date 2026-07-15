import { describe, expect, it, vi } from "vitest"
import type { Message } from "@/types/api"
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
    created_at: "2026-01-01T00:00:02Z",
    updated_at: "2026-01-01T00:00:02Z",
  },
]

describe("MessageList", () => {
  it("renders human and AI message content", () => {
    render(
      <MessageList
        messages={messages}
        onRegenerate={vi.fn()}
        isRegenerating={null}
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
      />,
    )

    const regenerateButton = screen.getByRole("button", { name: /^regenerate$/i })
    regenerateButton.click()

    expect(onRegenerate).toHaveBeenCalledWith("message-ai-completed")
  })
})
