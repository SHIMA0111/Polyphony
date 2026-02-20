import type {
  AuthResponse,
  Room,
  Message,
  MessagePage,
  AIMessageResponse,
  ModelListResponse,
} from "@/types/api"
import { mockApi } from "./mock-data"

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"
export const IS_MOCK = process.env.NEXT_PUBLIC_MOCK_API === "true"

export interface ApiClientInterface {
  setToken(token: string): void
  getToken(): string | null
  clearToken(): void
  register(
    email: string,
    username: string,
    password: string,
  ): Promise<AuthResponse>
  login(email: string, password: string): Promise<AuthResponse>
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
  private token: string | null = null

  setToken(token: string) {
    this.token = token
    if (typeof window !== "undefined") {
      localStorage.setItem("access_token", token)
    }
  }

  getToken(): string | null {
    if (this.token) return this.token
    if (typeof window !== "undefined") {
      this.token = localStorage.getItem("access_token")
    }
    return this.token
  }

  clearToken() {
    this.token = null
    if (typeof window !== "undefined") {
      localStorage.removeItem("access_token")
    }
  }

  private async request<T>(
    path: string,
    options: RequestInit = {},
  ): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...((options.headers as Record<string, string>) ?? {}),
    }

    const token = this.getToken()
    if (token) {
      headers["Authorization"] = `Bearer ${token}`
    }

    const res = await fetch(`${API_BASE_URL}${path}`, {
      ...options,
      headers,
    })

    if (!res.ok) {
      const error = await res
        .json()
        .catch(() => ({ message: "Unknown error" }))
      throw new Error(error.message ?? `Request failed: ${res.status}`)
    }

    if (res.status === 204) {
      return undefined as T
    }

    return res.json()
  }

  async register(
    email: string,
    username: string,
    password: string,
  ): Promise<AuthResponse> {
    return this.request<AuthResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, username, password }),
    })
  }

  async login(email: string, password: string): Promise<AuthResponse> {
    return this.request<AuthResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    })
  }

  async listRooms(): Promise<Room[]> {
    return this.request<Room[]>("/rooms")
  }

  async getRoom(roomId: string): Promise<Room> {
    return this.request<Room>(`/rooms/${roomId}`)
  }

  async createRoom(name: string, description: string): Promise<Room> {
    return this.request<Room>("/rooms", {
      method: "POST",
      body: JSON.stringify({ name, description }),
    })
  }

  async deleteRoom(roomId: string): Promise<void> {
    return this.request<void>(`/rooms/${roomId}`, { method: "DELETE" })
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
    return this.request<MessagePage>(
      `/rooms/${roomId}/messages${query ? `?${query}` : ""}`,
    )
  }

  async sendMessage(roomId: string, content: string): Promise<Message> {
    return this.request<Message>(`/rooms/${roomId}/messages`, {
      method: "POST",
      body: JSON.stringify({ content }),
    })
  }

  async sendAIMessage(
    roomId: string,
    content: string,
    model?: string,
  ): Promise<AIMessageResponse> {
    return this.request<AIMessageResponse>(`/rooms/${roomId}/messages/ai`, {
      method: "POST",
      body: JSON.stringify({ content, model }),
    })
  }

  async regenerateAIMessage(
    roomId: string,
    messageId: string,
    model?: string,
  ): Promise<Message> {
    return this.request<Message>(
      `/rooms/${roomId}/messages/${messageId}/regenerate`,
      {
        method: "POST",
        body: JSON.stringify({ model }),
      },
    )
  }

  async listModels(): Promise<ModelListResponse> {
    return this.request<ModelListResponse>("/models")
  }
}

class MockApiClient implements ApiClientInterface {
  private token: string | null = "mock-token"

  setToken(token: string) {
    this.token = token
  }
  getToken() {
    return this.token
  }
  clearToken() {
    this.token = null
  }

  register(
    email: string,
    username: string,
    _password: string,
  ): Promise<AuthResponse> {
    void email
    void username
    return mockApi.register()
  }
  login(_email: string, _password: string): Promise<AuthResponse> {
    return mockApi.login()
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
        { id: "gpt-5-mini", provider: "OpenAI" },
        { id: "gpt-5", provider: "OpenAI" },
      ],
    }
  }
}

export const apiClient: ApiClientInterface = IS_MOCK
  ? new MockApiClient()
  : new RealApiClient()
