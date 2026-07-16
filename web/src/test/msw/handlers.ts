import { http, HttpResponse } from "msw"
import type {
  AIMessageResponse,
  AuthResponse,
  Message,
  MessagePage,
  ModelListResponse,
  Room,
} from "@/types/api"

/**
 * MSW request handlers for the web frontend's unit/component test suite.
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the Go
 * API — the same path suffixes `web/src/lib/api.ts`'s `RealApiClient` already
 * sends today, just prefixed with `/api/proxy`. Do not point these at
 * `RealApiClient`'s current absolute `http://localhost:8080` base URL; that is
 * the direct-to-Go-API path Step 9 removes when it migrates call sites onto
 * this proxy.
 *
 * Shared by both the Node `setupServer` (used by Vitest, see `server.ts`) and
 * the browser `setupWorker` (see `browser.ts`).
 */

const fixtureRoom: Room = {
  id: "room-1",
  name: "General",
  description: "General discussion room",
  owner_id: "user-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

const fixtureRooms: Room[] = [
  fixtureRoom,
  {
    id: "room-2",
    name: "Random",
    description: "Off-topic chatter",
    owner_id: "user-1",
    created_at: "2026-01-02T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
]

const fixtureAuthResponse: AuthResponse = {
  access_token: "mock-access-token",
  token_type: "Bearer",
}

const fixtureHumanMessage: Message = {
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

const fixtureAiMessage: Message = {
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

const fixtureMessagePage: MessagePage = {
  messages: [fixtureHumanMessage, fixtureAiMessage],
  next_cursor: null,
}

const fixtureAiMessageResponse: AIMessageResponse = {
  user_message: fixtureHumanMessage,
  ai_message: fixtureAiMessage,
}

const fixtureModelListResponse: ModelListResponse = {
  models: [
    { id: "gpt-5-mini", name: "gpt-5-mini", provider: "OpenAI" },
    { id: "gpt-5", name: "gpt-5", provider: "OpenAI" },
  ],
}

export const handlers = [
  http.post("/api/proxy/auth/login", () => {
    return HttpResponse.json<AuthResponse>(fixtureAuthResponse)
  }),

  http.post("/api/proxy/auth/register", () => {
    return HttpResponse.json<AuthResponse>(fixtureAuthResponse)
  }),

  http.get("/api/proxy/rooms", () => {
    return HttpResponse.json<Room[]>(fixtureRooms)
  }),

  http.post("/api/proxy/rooms", () => {
    return HttpResponse.json<Room>(fixtureRoom, { status: 201 })
  }),

  http.get("/api/proxy/rooms/:roomId", ({ params }) => {
    return HttpResponse.json<Room>({ ...fixtureRoom, id: String(params.roomId) })
  }),

  http.delete("/api/proxy/rooms/:roomId", () => {
    return new HttpResponse(null, { status: 204 })
  }),

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
