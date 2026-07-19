import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { server } from "@/test/msw/server"
import { render, screen, waitFor, within } from "@/test/render"
import { fixtureRoom } from "@/features/rooms/api/handlers"
import type { Room } from "@/features/rooms/types"
import type { RoomRole } from "@/features/members/types"
import type { Message, MessagePage } from "@/features/messages/types"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { flattenMessagePages } from "@/features/messages/lib/flatten-message-pages"
import { MessageBubble } from "./MessageBubble"
import { MessageList } from "./MessageList"

/**
 * Component test for `MessageBubble`'s Step 38 per-message action menu:
 * exclude/include toggle (patches the message + shows/hides the visual
 * indicator), delete (behind a confirmation dialog, removing the message
 * from the rendered list on success), and role-gated menu visibility.
 *
 * `MessageBubble` reads the current user (`useSession`) and room role
 * (`useRoom`) itself rather than via props, so these tests drive that
 * through MSW response overrides (`server.use(...)`) instead of passing
 * anything extra as component props. Interactions use `@testing-library/
 * user-event` (rather than a bare DOM `.click()`) since Chakra v3's
 * Ark-UI-backed `Menu`/`Dialog` primitives rely on real pointer event
 * sequences to open — matching the existing convention in
 * `CreateRoomForm.test.tsx`.
 */

const baseMessage: Message = {
  id: "message-1",
  room_id: "room-1",
  sender_id: "user-1", // matches `fixtureSession`'s identity id
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
}

const noopProps = {
  onRegenerate: vi.fn(),
  isRegenerating: false,
  onRetry: vi.fn(),
}

/** Overrides the room fixture's role for the current test only. */
function useRoomWithRole(role: RoomRole) {
  server.use(
    http.get("/api/proxy/rooms/:roomId", ({ params }) => {
      return HttpResponse.json<Room>({
        ...fixtureRoom,
        id: String(params.roomId),
        role,
      })
    }),
  )
}

/**
 * Renders `baseMessage` through the *real* cache-subscribing pipeline
 * (`useMessages` -> `flattenMessagePages` -> `MessageList` -> `MessageGroup`
 * -> `MessageBubble`) rather than passing a static `message` prop straight
 * to `MessageBubble`. This matters here specifically because
 * `useDeleteMessage`/`useUpdateMessageExclude`'s `onSuccess` write into the
 * shared `["rooms", roomId, "messages"]` query cache — in the real app,
 * `ChatRoom`'s own `useMessages` subscription is what turns that cache write
 * into a new `messages` prop flowing back down; a bare `<MessageBubble
 * message={...} />` render has no such subscriber, so its prop would never
 * reflect the mutation's own result.
 */
function MessageListHarness({ roomId }: { roomId: string }) {
  const query = useMessages(roomId)
  if (!query.data) return null
  const messages = flattenMessagePages(query.data.pages)
  return (
    <MessageList
      messages={messages}
      onRegenerate={noopProps.onRegenerate}
      isRegenerating={null}
      onRetry={noopProps.onRetry}
      hasNextPage={false}
      isFetchingNextPage={false}
      fetchNextPage={vi.fn()}
      pageCount={1}
    />
  )
}

/**
 * Component coverage for Step 54's three AI-bubble states (thinking / live
 * streaming / finalized), rendered directly via `<MessageBubble>` rather
 * than through `MessageListHarness` -- these tests only need to observe
 * `MessageBubble`'s own conditional rendering for a given `message.status`,
 * not the query-cache merge behavior `merge-message-event.test.ts` already
 * covers.
 */
describe("MessageBubble streaming states", () => {
  const baseAiMessage: Message = {
    id: "ai-1",
    room_id: "room-1",
    sender_id: null,
    content: "",
    type: "ai",
    status: "sending",
    sequence: -1,
    in_response_to_message_id: "message-1",
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  }

  it('renders the thinking indicator for status "sending" with no content', async () => {
    render(<MessageBubble message={baseAiMessage} {...noopProps} />)

    expect(await screen.findByLabelText("AI is thinking")).toBeInTheDocument()
  })

  it('renders the thinking indicator for status "streaming" before any content has arrived', async () => {
    render(
      <MessageBubble
        message={{ ...baseAiMessage, status: "streaming", content: "" }}
        {...noopProps}
      />,
    )

    expect(await screen.findByLabelText("AI is thinking")).toBeInTheDocument()
  })

  it('renders growing plain text and a "Streaming…" status once content has arrived while streaming', async () => {
    render(
      <MessageBubble
        message={{ ...baseAiMessage, status: "streaming", content: "Hello there" }}
        {...noopProps}
      />,
    )

    expect(await screen.findByText("Hello there")).toBeInTheDocument()
    expect(screen.getByText("Streaming…")).toBeInTheDocument()
    expect(screen.queryByLabelText("AI is thinking")).not.toBeInTheDocument()
  })

  it("renders the finalized markdown content and a timestamp once completed", async () => {
    render(
      <MessageBubble
        message={{ ...baseAiMessage, status: "completed", content: "Hello there" }}
        {...noopProps}
      />,
    )

    expect(await screen.findByText("Hello there")).toBeInTheDocument()
    expect(screen.queryByText("Streaming…")).not.toBeInTheDocument()
    expect(screen.queryByText("Sending…")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("AI is thinking")).not.toBeInTheDocument()
  })

  // Carryover from wave-7 review: RegenerateAIMessage now rejects a target
  // AI response that is still mid-stream (see server/internal/usecase/
  // message/usecase.go), so the client should not let a user trigger that
  // request in the first place while the message is still streaming.
  it("disables the Regenerate button while the AI message is still streaming", async () => {
    render(
      <MessageBubble
        message={{ ...baseAiMessage, status: "streaming", content: "Hello there" }}
        {...noopProps}
      />,
    )

    expect(await screen.findByRole("button", { name: /regenerate/i })).toBeDisabled()
  })

  // Step 60 hardening: `useSendAIMessage` (Step 29) optimistically appends
  // the AI placeholder itself with `status: "sending"` before the request
  // has round-tripped -- that window is distinct from `"streaming"` (no
  // `token_chunk` has necessarily arrived yet, or ever will if the model
  // doesn't stream), but a Regenerate click at that point targets a message
  // that isn't finalized any more than a streaming one is, so it must be
  // disabled too.
  it('disables the Regenerate button while the AI message is still the optimistic "sending" placeholder', async () => {
    render(<MessageBubble message={baseAiMessage} {...noopProps} />)

    expect(await screen.findByRole("button", { name: /regenerate/i })).toBeDisabled()
  })

  it("keeps the Regenerate button enabled once the AI message has finalized", async () => {
    render(
      <MessageBubble
        message={{ ...baseAiMessage, status: "completed", content: "Hello there" }}
        {...noopProps}
      />,
    )

    expect(await screen.findByRole("button", { name: /regenerate/i })).toBeEnabled()
  })
})

describe("MessageBubble action menu", () => {
  it("toggles exclude_from_ai and updates the visual indicator once the mutation resolves", async () => {
    useRoomWithRole("member")
    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [baseMessage],
          next_cursor: null,
        })
      }),
      http.patch(
        "/api/proxy/rooms/:roomId/messages/:messageId",
        async ({ request }) => {
          const body = (await request.json()) as { exclude_from_ai: boolean }
          return HttpResponse.json<Message>({
            ...baseMessage,
            exclude_from_ai: body.exclude_from_ai,
          })
        },
      ),
    )
    const user = userEvent.setup()

    render(<MessageListHarness roomId="room-1" />)

    await screen.findByText("Hello, AI!")
    expect(
      screen.queryByLabelText("Excluded from AI context"),
    ).not.toBeInTheDocument()

    await user.click(
      await screen.findByRole("button", { name: "Message actions" }),
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Exclude from AI" }),
    )

    await waitFor(() =>
      expect(
        screen.getByLabelText("Excluded from AI context"),
      ).toBeInTheDocument(),
    )
  })

  it("shows 'Include in AI' for an already-excluded message", async () => {
    useRoomWithRole("member")
    const user = userEvent.setup()

    render(
      <MessageBubble
        message={{ ...baseMessage, exclude_from_ai: true }}
        {...noopProps}
      />,
    )

    await user.click(
      await screen.findByRole("button", { name: "Message actions" }),
    )

    expect(
      await screen.findByRole("menuitem", { name: "Include in AI" }),
    ).toBeInTheDocument()
    expect(
      screen.getByLabelText("Excluded from AI context"),
    ).toBeInTheDocument()
  })

  it("hides the exclude toggle (but keeps delete, since the caller owns the message) for a guest role", async () => {
    useRoomWithRole("guest")
    const user = userEvent.setup()

    render(<MessageBubble message={baseMessage} {...noopProps} />)

    await user.click(
      await screen.findByRole("button", { name: "Message actions" }),
    )

    expect(
      screen.queryByRole("menuitem", { name: "Exclude from AI" }),
    ).not.toBeInTheDocument()
    expect(
      await screen.findByRole("menuitem", { name: "Delete message" }),
    ).toBeInTheDocument()
  })

  it("hides the entire menu for a reader viewing another user's message", async () => {
    useRoomWithRole("reader")

    render(
      <MessageBubble
        message={{ ...baseMessage, sender_id: "someone-else" }}
        {...noopProps}
      />,
    )

    // Give any async session/room queries a chance to resolve before
    // asserting the menu trigger never appears.
    await waitFor(() =>
      expect(screen.queryByText(/Hello, AI!/)).toBeInTheDocument(),
    )
    expect(
      screen.queryByRole("button", { name: "Message actions" }),
    ).not.toBeInTheDocument()
  })

  it("requires confirmation before calling deleteMessage, and does nothing if cancelled", async () => {
    useRoomWithRole("admin")
    let deleteCalled = false
    server.use(
      http.delete("/api/proxy/rooms/:roomId/messages/:messageId", () => {
        deleteCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const user = userEvent.setup()

    render(<MessageBubble message={baseMessage} {...noopProps} />)

    await user.click(
      await screen.findByRole("button", { name: "Message actions" }),
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Delete message" }),
    )

    const dialog = await screen.findByRole("alertdialog")
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))

    await waitFor(() =>
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument(),
    )
    expect(deleteCalled).toBe(false)
    expect(screen.getByText("Hello, AI!")).toBeInTheDocument()
  })

  it("removes the message from the rendered list once the delete is confirmed", async () => {
    useRoomWithRole("admin")
    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [baseMessage],
          next_cursor: null,
        })
      }),
      http.delete("/api/proxy/rooms/:roomId/messages/:messageId", () => {
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const user = userEvent.setup()

    render(<MessageListHarness roomId="room-1" />)

    await screen.findByText("Hello, AI!")

    await user.click(
      await screen.findByRole("button", { name: "Message actions" }),
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Delete message" }),
    )

    const dialog = await screen.findByRole("alertdialog")
    await user.click(
      within(dialog).getByRole("button", { name: "Delete" }),
    )

    await waitFor(() =>
      expect(screen.queryByText("Hello, AI!")).not.toBeInTheDocument(),
    )
  })
})
