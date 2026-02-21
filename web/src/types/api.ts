export interface AuthResponse {
  access_token: string
  token_type: string
}

export interface User {
  id: string
  email: string
  username: string
  created_at: string
  updated_at: string
}

export interface Room {
  id: string
  name: string
  description: string
  owner_id: string
  created_at: string
  updated_at: string
}

export type MessageType = "human" | "ai"
export type MessageStatus = "completed" | "failed"

export interface Message {
  id: string
  room_id: string
  sender_id: string | null
  content: string
  type: MessageType
  status: MessageStatus
  sequence: number
  created_at: string
  updated_at: string
}

export interface MessagePage {
  messages: Message[]
  next_cursor: string | null
}

export interface AIMessageResponse {
  user_message: Message
  ai_message: Message
}

export interface ModelInfo {
  id: string
  name: string
  provider: string
}

export interface ModelListResponse {
  models: ModelInfo[]
}

export interface ApiError {
  message: string
}
