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
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

const excludedMessage: Message = {
  ...humanMessage,
  id: "message-2",
  content: "Excluded message",
  exclude_from_ai: true,
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
