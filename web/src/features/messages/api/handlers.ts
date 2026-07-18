import { http, HttpResponse } from "msw"
import type {
  AIMessageResponse,
  Message,
  MessagePage,
  ModelListResponse,
} from "../types"

/**
 * MSW request handlers for the messages feature, shared by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`) and the browser
 * `setupWorker` (`src/test/msw/browser.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API.
 */

export const fixtureHumanMessage: Message = {
  id: "message-1",
  room_id: "room-1",
  sender_id: "user-1",
  content: "Hello, AI!",
  type: "human",
  status: "completed",
  sequence: 1,
  in_response_to_message_id: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

export const fixtureAiMessage: Message = {
  id: "message-2",
  room_id: "room-1",
  sender_id: null,
  content: "Hello! How can I help you today?",
  type: "ai",
  status: "completed",
  sequence: 2,
  in_response_to_message_id: "message-1",
  created_at: "2026-01-01T00:00:01Z",
  updated_at: "2026-01-01T00:00:01Z",
}

export const fixtureMessagePage: MessagePage = {
  // Descending order, matching the real API's contract; consumers reverse
  // this for display (see `../api/get-messages.ts`).
  messages: [fixtureAiMessage, fixtureHumanMessage],
  next_cursor: null,
}

export const fixtureAiMessageResponse: AIMessageResponse = {
  user_message: fixtureHumanMessage,
  ai_message: fixtureAiMessage,
}

export const fixtureModelListResponse: ModelListResponse = {
  models: [
    {
      id: "gpt-5-mini",
      name: "gpt-5-mini",
      provider: "OpenAI",
      context_window: 272_000,
      input_price_per_million_tokens: 0.25,
      output_price_per_million_tokens: 2.0,
      supports_image_input: true,
    },
    {
      id: "gpt-5",
      name: "gpt-5",
      provider: "OpenAI",
      context_window: 272_000,
      input_price_per_million_tokens: 1.25,
      output_price_per_million_tokens: 10.0,
      supports_image_input: true,
    },
    {
      id: "claude-sonnet-4-6",
      name: "Claude Sonnet 4.6",
      provider: "Anthropic",
      context_window: 200_000,
      input_price_per_million_tokens: 3.0,
      output_price_per_million_tokens: 15.0,
      supports_image_input: true,
    },
  ],
}

export const messagesHandlers = [
  http.get("/api/proxy/rooms/:roomId/messages", ({ request }) => {
    const url = new URL(request.url)
    const cursor = url.searchParams.get("cursor")

    // The fixture only models a single page of room history; a `cursor`
    // query param (as sent by `useMessages`'s `useInfiniteQuery` when
    // fetching an older page via `fetchNextPage`) has no further history to
    // return.
    if (cursor) {
      return HttpResponse.json<MessagePage>({ messages: [], next_cursor: null })
    }

    return HttpResponse.json<MessagePage>(fixtureMessagePage)
  }),

  http.post("/api/proxy/rooms/:roomId/messages", () => {
    return HttpResponse.json<Message>(fixtureHumanMessage, { status: 201 })
  }),

  http.post("/api/proxy/rooms/:roomId/messages/ai", () => {
    return HttpResponse.json<AIMessageResponse>(fixtureAiMessageResponse, {
      status: 201,
    })
  }),

  http.post(
    "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
    () => {
      // Deliberately keep `fixtureAiMessage`'s own id rather than echoing
      // back `params.messageId`: a regenerated message is a distinct AI
      // message (new id, new content) that replaces/supersedes the one at
      // `:messageId`, not the same message mutated in place. Overriding the
      // id here would make the fixture indistinguishable from the message
      // being regenerated, masking bugs where a caller conflates the two
      // identities.
      return HttpResponse.json<Message>(fixtureAiMessage)
    },
  ),

  http.get("/api/proxy/models", () => {
    return HttpResponse.json<ModelListResponse>(fixtureModelListResponse)
  }),
]
