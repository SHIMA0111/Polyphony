import { describe, expect, it, vi } from "vitest"
import type { Message } from "@/features/messages/types"
import { render, screen } from "@/test/render"
import { MessageGroup } from "./MessageGroup"

/**
 * Pure-component test for `MessageGroup`'s sender label (product-bug fix:
 * every human message used to be labeled "You" unconditionally, mislabeling
 * every *other* member's message too). Covers the viewer's own message
 * ("You"), another member's message (their username via `senderUsernames`),
 * a sender who has since left the room (id-based fallback), and that AI
 * groups are unaffected by any of this.
 */

const noopProps = {
  onRegenerate: vi.fn(),
  isRegenerating: null,
  onRetry: vi.fn(),
}

function makeMessage(overrides: Partial<Message> & Pick<Message, "id">): Message {
  return {
    room_id: "room-1",
    sender_id: "user-1",
    content: "hello",
    type: "human",
    status: "completed",
    sequence: 1,
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T12:00:00.000Z",
    updated_at: "2026-01-01T12:00:00.000Z",
    ...overrides,
  }
}

describe("MessageGroup sender label", () => {
  it('labels the viewer\'s own message group "You"', () => {
    render(
      <MessageGroup
        type="human"
        senderId="user-1"
        messages={[makeMessage({ id: "m1", sender_id: "user-1" })]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser", "user-2": "alice" }}
        {...noopProps}
      />,
    )

    expect(screen.getByText("You")).toBeInTheDocument()
    expect(screen.queryByText("testuser")).not.toBeInTheDocument()
  })

  it("labels another member's message group with their username, not \"You\"", () => {
    render(
      <MessageGroup
        type="human"
        senderId="user-2"
        messages={[makeMessage({ id: "m2", sender_id: "user-2" })]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser", "user-2": "alice" }}
        {...noopProps}
      />,
    )

    expect(screen.getByText("alice")).toBeInTheDocument()
    expect(screen.queryByText("You")).not.toBeInTheDocument()
  })

  it("falls back to a short id-based label for a sender missing from senderUsernames (e.g. they left the room)", () => {
    render(
      <MessageGroup
        type="human"
        senderId="user-departed-123"
        messages={[makeMessage({ id: "m3", sender_id: "user-departed-123" })]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser" }}
        {...noopProps}
      />,
    )

    expect(screen.getByText(/^User user-d/)).toBeInTheDocument()
    expect(screen.queryByText("You")).not.toBeInTheDocument()
  })

  it("renders every human message group's viewer/other distinction correctly within the same room", () => {
    // Two consecutive human groups (different senders never merge into one
    // group -- see `groupMessagesForDisplay`) proves the label is resolved
    // per-group from that group's own senderId, not e.g. cached/stale from
    // a previous render.
    const { rerender } = render(
      <MessageGroup
        type="human"
        senderId="user-1"
        messages={[makeMessage({ id: "m4", sender_id: "user-1" })]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser", "user-2": "alice" }}
        {...noopProps}
      />,
    )
    expect(screen.getByText("You")).toBeInTheDocument()

    rerender(
      <MessageGroup
        type="human"
        senderId="user-2"
        messages={[makeMessage({ id: "m5", sender_id: "user-2" })]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser", "user-2": "alice" }}
        {...noopProps}
      />,
    )
    expect(screen.getByText("alice")).toBeInTheDocument()
    expect(screen.queryByText("You")).not.toBeInTheDocument()
  })

  it('leaves AI groups labeled "AI" regardless of currentUserId/senderUsernames', () => {
    render(
      <MessageGroup
        type="ai"
        senderId={null}
        messages={[
          makeMessage({
            id: "m6",
            sender_id: null,
            type: "ai",
            content: "Hello! How can I help?",
          }),
        ]}
        currentUserId="user-1"
        senderUsernames={{ "user-1": "testuser" }}
        {...noopProps}
      />,
    )

    expect(screen.getByText("AI")).toBeInTheDocument()
  })
})
