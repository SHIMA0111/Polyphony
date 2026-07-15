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
    { id: "gpt-5-mini", name: "gpt-5-mini", provider: "OpenAI" },
    { id: "gpt-5", name: "gpt-5", provider: "OpenAI" },
  ],
}

export const messagesHandlers = [
  http.get("/api/proxy/rooms/:roomId/messages", () => {
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
    ({ params }) => {
      return HttpResponse.json<Message>({
        ...fixtureAiMessage,
        id: String(params.messageId),
      })
    },
  ),

  http.get("/api/proxy/models", () => {
    return HttpResponse.json<ModelListResponse>(fixtureModelListResponse)
  }),
]
