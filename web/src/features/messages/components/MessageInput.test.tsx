import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { server } from "@/test/msw/server"
import { render, screen, waitFor } from "@/test/render"
import type { Message, ModelInfo, TokenEstimateResponse } from "@/features/messages/types"
import { MessageInput } from "./MessageInput"

/**
 * Component test for `MessageInput`'s Step 38 live token meter: debounced
 * display of the estimated count, and that the outgoing estimate payload
 * excludes `is_deleted`/`exclude_from_ai` messages per the doc's meter
 * payload filtering requirement.
 */

const models: ModelInfo[] = [
  {
    id: "gpt-5-mini",
    name: "gpt-5-mini",
    provider: "OpenAI",
    context_window: 128000,
    input_price_per_million_tokens: 0.25,
    output_price_per_million_tokens: 2,
    supports_image_input: false,
  },
]

const humanMessage: Message = {
  id: "message-1",
  room_id: "room-1",
  sender_id: "user-1",
  content: "Included message",
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

const excludedMessage: Message = {
  ...humanMessage,
  id: "message-2",
  content: "Excluded message",
  exclude_from_ai: true,
  used_context_summary: false,
}

const deletedMessage: Message = {
  ...humanMessage,
  id: "message-3",
  content: "Deleted message",
  is_deleted: true,
}

const noopHandlers = {
  onSend: vi.fn().mockResolvedValue(undefined),
  onSendWithAI: vi.fn().mockResolvedValue(undefined),
}

describe("MessageInput token meter", () => {
  it("shows the debounced estimated token count returned by /tokens/estimate", async () => {
    server.use(
      http.post("/api/proxy/tokens/estimate", () => {
        return HttpResponse.json<TokenEstimateResponse>({
          model: "gpt-5-mini",
          estimated_tokens: 123,
        })
      }),
    )

    render(
      <MessageInput
        roomId="room-1"
        {...noopHandlers}
        models={models}
        messages={[humanMessage]}
      />,
    )

    // Not shown before the debounce fires.
    expect(screen.queryByText(/tokens/)).not.toBeInTheDocument()

    await waitFor(
      () => expect(screen.getByText("~123 tokens")).toBeInTheDocument(),
      { timeout: 5000 },
    )
  })

  it("excludes is_deleted and exclude_from_ai messages from the estimate payload", async () => {
    let capturedContents: string[] = []
    server.use(
      http.post("/api/proxy/tokens/estimate", async ({ request }) => {
        const body = (await request.json()) as {
          messages: { role: string; content: string }[]
        }
        capturedContents = body.messages.map((m) => m.content)
        return HttpResponse.json<TokenEstimateResponse>({
          model: "gpt-5-mini",
          estimated_tokens: 7,
        })
      }),
    )

    render(
      <MessageInput
        roomId="room-1"
        {...noopHandlers}
        models={models}
        messages={[humanMessage, excludedMessage, deletedMessage]}
      />,
    )

    await waitFor(() => expect(screen.getByText("~7 tokens")).toBeInTheDocument(), {
      timeout: 5000,
    })

    expect(capturedContents).toEqual(["Included message"])
  })
})

/**
 * Component-level tests for `MessageInput`'s Step 48 `aiError` prop: an
 * additive, default-preserving optional prop rendering a short inline error
 * line (with a link to `/billing/usage`) when set, and cleared locally on
 * the next input change — none of this changes `onSend`/`onSendWithAI`'s
 * existing try/finally structure.
 */
describe("MessageInput aiError", () => {
  it("renders no inline error by default", () => {
    render(
      <MessageInput
        roomId="room-1"
        onSend={vi.fn()}
        onSendWithAI={vi.fn()}
        models={models}
      />,
    )

    expect(screen.queryByRole("alert")).not.toBeInTheDocument()
  })

  it("renders the inline error with a link to /billing/usage when aiError is set", () => {
    render(
      <MessageInput
        roomId="room-1"
        onSend={vi.fn()}
        onSendWithAI={vi.fn()}
        models={models}
        aiError="Insufficient token balance."
      />,
    )

    const alert = screen.getByRole("alert")
    expect(alert).toHaveTextContent("Insufficient token balance.")
    expect(screen.getByRole("link", { name: "Usage" })).toHaveAttribute(
      "href",
      "/billing/usage",
    )
  })

  it("dismisses the inline error locally once the user types again", async () => {
    const user = userEvent.setup()
    render(
      <MessageInput
        roomId="room-1"
        onSend={vi.fn()}
        onSendWithAI={vi.fn()}
        models={models}
        aiError="Insufficient token balance."
      />,
    )

    expect(screen.getByRole("alert")).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText("Ask me anything..."), "x")

    expect(screen.queryByRole("alert")).not.toBeInTheDocument()
  })
})

/**
 * Component-level tests for `MessageInput`'s Step 47 private-mode toggle:
 * off by default, toggles on click, forwards the correct boolean as
 * `onSendWithAI`'s 4th argument, has no effect on the plain `onSend` path,
 * resets to off after a successful "Send with AI", and is omitted entirely
 * when `canInvokeAI` is `false` (mirroring the "Send with AI" button itself).
 */
describe("MessageInput private mode toggle", () => {
  it("is off by default and toggles on when clicked", async () => {
    const user = userEvent.setup()
    render(
      <MessageInput roomId="room-1" {...noopHandlers} models={models} />,
    )

    const toggle = screen.getByRole("button", { name: "Private mode off" })
    expect(toggle).toHaveAttribute("aria-pressed", "false")

    await user.click(toggle)

    expect(
      screen.getByRole("button", { name: "Private mode on" }),
    ).toHaveAttribute("aria-pressed", "true")
  })

  it("passes isPrivate=false to onSendWithAI by default", async () => {
    const onSendWithAI = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()

    render(
      <MessageInput
        roomId="room-1"
        onSend={vi.fn()}
        onSendWithAI={onSendWithAI}
        models={models}
      />,
    )

    await user.type(
      screen.getByPlaceholderText("Ask me anything..."),
      "Public question",
    )
    await user.click(screen.getByRole("button", { name: "Send with AI" }))

    await waitFor(() =>
      expect(onSendWithAI).toHaveBeenCalledWith(
        "Public question",
        "gpt-5-mini",
        [],
        false,
      ),
    )
  })

  it("passes isPrivate=true when the toggle is on, then resets the toggle to off after the send resolves", async () => {
    const onSendWithAI = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()

    render(
      <MessageInput
        roomId="room-1"
        onSend={vi.fn()}
        onSendWithAI={onSendWithAI}
        models={models}
      />,
    )

    await user.click(screen.getByRole("button", { name: "Private mode off" }))
    await user.type(
      screen.getByPlaceholderText("Ask me anything..."),
      "Secret question",
    )
    await user.click(screen.getByRole("button", { name: "Send with AI" }))

    await waitFor(() =>
      expect(onSendWithAI).toHaveBeenCalledWith(
        "Secret question",
        "gpt-5-mini",
        [],
        true,
      ),
    )

    // Opt-in per message, not sticky: back to off once the send resolves.
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Private mode off" }),
      ).toBeInTheDocument(),
    )
  })

  it("has no effect on the plain Send path", async () => {
    const onSend = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()

    render(
      <MessageInput
        roomId="room-1"
        onSend={onSend}
        onSendWithAI={vi.fn().mockResolvedValue(undefined)}
        models={models}
      />,
    )

    await user.click(screen.getByRole("button", { name: "Private mode off" }))
    await user.type(
      screen.getByPlaceholderText("Ask me anything..."),
      "Just a normal message",
    )
    await user.click(screen.getByRole("button", { name: "Send" }))

    await waitFor(() =>
      expect(onSend).toHaveBeenCalledWith("Just a normal message", []),
    )
  })

  it("omits the private-mode toggle when canInvokeAI is false", () => {
    render(
      <MessageInput
        roomId="room-1"
        {...noopHandlers}
        models={models}
        canInvokeAI={false}
      />,
    )

    expect(
      screen.queryByRole("button", { name: /Private mode/ }),
    ).not.toBeInTheDocument()
  })
})

/**
 * Step 34's review fix: "Send with AI" must have its own disabled condition
 * that also accounts for there being no model to send to — sharing the
 * plain-send button's `isDisabled` alone would let it render clickable (and
 * silently no-op via `handleSendAI`'s own `!effectiveModel` guard) whenever
 * `models` is empty, e.g. before the model list has loaded or in a room
 * with no configured models.
 */
describe("MessageInput AI-send button model gating", () => {
  it("disables 'Send with AI' when there is text but no model available", async () => {
    const user = userEvent.setup()
    render(<MessageInput roomId="room-1" {...noopHandlers} models={[]} />)

    await user.type(
      screen.getByPlaceholderText("Ask me anything..."),
      "Hello with no model",
    )

    expect(screen.getByRole("button", { name: "Send with AI" })).toBeDisabled()
  })

  it("enables 'Send with AI' once a model is available", async () => {
    const user = userEvent.setup()
    render(<MessageInput roomId="room-1" {...noopHandlers} models={models} />)

    await user.type(
      screen.getByPlaceholderText("Ask me anything..."),
      "Hello with a model",
    )

    expect(
      screen.getByRole("button", { name: "Send with AI" }),
    ).not.toBeDisabled()
  })
})
