import type {
  Room,
  Message,
  MessagePage,
  AIMessageResponse,
  ModelListResponse,
} from "@/types/api"
import { apiRequest, authRequest } from "./http-client"
import { mockApi } from "./mock-data"

export const IS_MOCK = process.env.NEXT_PUBLIC_MOCK_API === "true"

export interface ApiClientInterface {
  register(email: string, username: string, password: string): Promise<void>
  login(email: string, password: string): Promise<void>
  logout(): Promise<void>
  listRooms(): Promise<Room[]>
  getRoom(roomId: string): Promise<Room>
  createRoom(name: string, description: string): Promise<Room>
  deleteRoom(roomId: string): Promise<void>
  listMessages(
    roomId: string,
    cursor?: string,
    limit?: number,
  ): Promise<MessagePage>
  sendMessage(roomId: string, content: string): Promise<Message>
  sendAIMessage(
    roomId: string,
    content: string,
    model?: string,
  ): Promise<AIMessageResponse>
  regenerateAIMessage(
    roomId: string,
    messageId: string,
    model?: string,
  ): Promise<Message>
  listModels(): Promise<ModelListResponse>
}

class RealApiClient implements ApiClientInterface {
  async register(
    email: string,
    username: string,
    password: string,
  ): Promise<void> {
    await authRequest<{ ok: true }>("/register", {
      method: "POST",
      body: JSON.stringify({ email, username, password }),
    })
  }

  async login(email: string, password: string): Promise<void> {
    await authRequest<{ ok: true }>("/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    })
  }

  async logout(): Promise<void> {
    await authRequest<{ ok: true }>("/logout", { method: "POST" })
  }

  async listRooms(): Promise<Room[]> {
    return apiRequest<Room[]>("/rooms")
  }

  async getRoom(roomId: string): Promise<Room> {
    return apiRequest<Room>(`/rooms/${roomId}`)
  }

  async createRoom(name: string, description: string): Promise<Room> {
    return apiRequest<Room>("/rooms", {
      method: "POST",
      body: JSON.stringify({ name, description }),
    })
  }

  async deleteRoom(roomId: string): Promise<void> {
    return apiRequest<void>(`/rooms/${roomId}`, { method: "DELETE" })
  }

  async listMessages(
    roomId: string,
    cursor?: string,
    limit?: number,
  ): Promise<MessagePage> {
    const params = new URLSearchParams()
    if (cursor) params.set("cursor", cursor)
    if (limit) params.set("limit", limit.toString())
    const query = params.toString()
    return apiRequest<MessagePage>(
      `/rooms/${roomId}/messages${query ? `?${query}` : ""}`,
    )
  }

  async sendMessage(roomId: string, content: string): Promise<Message> {
    return apiRequest<Message>(`/rooms/${roomId}/messages`, {
      method: "POST",
      body: JSON.stringify({ content }),
    })
  }

  async sendAIMessage(
    roomId: string,
    content: string,
    model?: string,
  ): Promise<AIMessageResponse> {
    return apiRequest<AIMessageResponse>(`/rooms/${roomId}/messages/ai`, {
      method: "POST",
      body: JSON.stringify({ content, model }),
    })
  }

  async regenerateAIMessage(
    roomId: string,
    messageId: string,
    model?: string,
  ): Promise<Message> {
    return apiRequest<Message>(
      `/rooms/${roomId}/messages/${messageId}/regenerate`,
      {
        method: "POST",
        body: JSON.stringify({ model }),
      },
    )
  }

  async listModels(): Promise<ModelListResponse> {
    return apiRequest<ModelListResponse>("/models")
  }
}

class MockApiClient implements ApiClientInterface {
  async register(
    email: string,
    username: string,
    _password: string,
  ): Promise<void> {
    void email
    void username
    await mockApi.register()
  }
  async login(_email: string, _password: string): Promise<void> {
    await mockApi.login()
  }
  async logout(): Promise<void> {
    await mockApi.logout()
  }
  listRooms(): Promise<Room[]> {
    return mockApi.listRooms()
  }
  getRoom(roomId: string): Promise<Room> {
    return mockApi.getRoom(roomId)
  }
  createRoom(name: string, description: string): Promise<Room> {
    return mockApi.createRoom(name, description)
  }
  deleteRoom(roomId: string): Promise<void> {
    return mockApi.deleteRoom(roomId)
  }
  listMessages(roomId: string): Promise<MessagePage> {
    return mockApi.listMessages(roomId)
  }
  sendMessage(roomId: string, content: string): Promise<Message> {
    return mockApi.sendMessage(roomId, content)
  }
  sendAIMessage(roomId: string, content: string): Promise<AIMessageResponse> {
    return mockApi.sendAIMessage(roomId, content)
  }
  regenerateAIMessage(roomId: string, messageId: string): Promise<Message> {
    return mockApi.regenerateAIMessage(roomId, messageId)
  }
  async listModels(): Promise<ModelListResponse> {
    return {
      models: [
        { id: "gpt-5-mini", name: "gpt-5-mini", provider: "OpenAI" },
        { id: "gpt-5", name: "gpt-5", provider: "OpenAI" },
      ],
    }
  }
}

export const apiClient: ApiClientInterface = IS_MOCK
  ? new MockApiClient()
  : new RealApiClient()
