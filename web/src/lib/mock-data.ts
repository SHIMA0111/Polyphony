import type { Room, Message, MessagePage, AIMessageResponse } from "@/types/api"

const MOCK_USER_ID = "00000000-0000-0000-0000-000000000001"

const MOCK_ROOMS: Room[] = [
  {
    id: "room-1",
    name: "Product Strategy",
    description:
      "Discuss product roadmap and feature planning with AI assistance",
    owner_id: MOCK_USER_ID,
    created_at: "2024-01-15T10:00:00Z",
    updated_at: "2024-01-15T10:00:00Z",
  },
  {
    id: "room-2",
    name: "Engineering Team",
    description:
      "Technical discussions, code reviews, and architectural decisions",
    owner_id: MOCK_USER_ID,
    created_at: "2024-01-20T10:00:00Z",
    updated_at: "2024-01-20T10:00:00Z",
  },
  {
    id: "room-3",
    name: "Marketing Campaign",
    description:
      "Brainstorm and refine marketing strategies with AI insights",
    owner_id: MOCK_USER_ID,
    created_at: "2024-02-01T10:00:00Z",
    updated_at: "2024-02-01T10:00:00Z",
  },
  {
    id: "room-4",
    name: "Customer Research",
    description: "Analyze customer feedback and identify patterns",
    owner_id: MOCK_USER_ID,
    created_at: "2024-02-05T10:00:00Z",
    updated_at: "2024-02-05T10:00:00Z",
  },
]

const MOCK_MESSAGES: Message[] = [
  {
    id: "msg-1",
    room_id: "room-1",
    sender_id: MOCK_USER_ID,
    content:
      "Hey team! Can we brainstorm some ideas for our Q2 product launch?",
    type: "human",
    status: "completed",
    sequence: 1,
    created_at: "2024-02-19T10:30:00Z",
    updated_at: "2024-02-19T10:30:00Z",
  },
  {
    id: "msg-2",
    room_id: "room-1",
    sender_id: null,
    content:
      "I'd be happy to help with Q2 launch brainstorming! Here are some ideas to consider:\n\n1. **Early Access Program** - Create exclusivity and buzz by offering select customers early access\n2. **Content Marketing Campaign** - Develop case studies and success stories from beta users\n3. **Partnership Announcements** - Time strategic partnerships to coincide with launch\n4. **Virtual Launch Event** - Host an interactive webinar showcasing key features\n\nWhich of these directions resonates most with your team's goals?",
    type: "ai",
    status: "completed",
    sequence: 2,
    created_at: "2024-02-19T10:30:15Z",
    updated_at: "2024-02-19T10:30:15Z",
  },
  {
    id: "msg-3",
    room_id: "room-1",
    sender_id: MOCK_USER_ID,
    content:
      "I really like the early access program idea. How should we structure it?",
    type: "human",
    status: "completed",
    sequence: 3,
    created_at: "2024-02-19T10:32:00Z",
    updated_at: "2024-02-19T10:32:00Z",
  },
  {
    id: "msg-4",
    room_id: "room-1",
    sender_id: null,
    content:
      "Great choice! Here's a structured approach for your early access program:\n\n**Phase 1: Selection (Week 1-2)**\n- Define ideal customer profile\n- Reach out to 50-100 engaged users from your waitlist\n- Aim for diverse use cases to get varied feedback\n\n**Phase 2: Onboarding (Week 3)**\n- Personalized onboarding calls\n- Dedicated Slack channel for participants\n- Share roadmap and gather input\n\n**Phase 3: Iteration (Week 4-6)**\n- Weekly check-ins and surveys\n- Prioritize bug fixes and UX improvements\n- Document success stories\n\nWould you like me to elaborate on any of these phases?",
    type: "ai",
    status: "completed",
    sequence: 4,
    created_at: "2024-02-19T10:32:30Z",
    updated_at: "2024-02-19T10:32:30Z",
  },
  {
    id: "msg-5",
    room_id: "room-1",
    sender_id: MOCK_USER_ID,
    content:
      "This is perfect! Can you draft an invitation email for the early access program?",
    type: "human",
    status: "completed",
    sequence: 5,
    created_at: "2024-02-19T10:35:00Z",
    updated_at: "2024-02-19T10:35:00Z",
  },
  {
    id: "msg-6",
    room_id: "room-1",
    sender_id: null,
    content: "",
    type: "ai",
    status: "failed",
    sequence: 6,
    created_at: "2024-02-19T10:35:45Z",
    updated_at: "2024-02-19T10:35:45Z",
  },
]

let mockRooms = [...MOCK_ROOMS]
let mockMessages = [...MOCK_MESSAGES]
let nextSeq = 7

function delay(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export const mockApi = {
  async register(): Promise<void> {
    await delay(500)
  },

  async login(): Promise<void> {
    await delay(500)
  },

  async logout(): Promise<void> {
    await delay(100)
  },

  async listRooms(): Promise<Room[]> {
    await delay(300)
    return mockRooms
  },

  async getRoom(roomId: string): Promise<Room> {
    await delay(200)
    const room = mockRooms.find((r) => r.id === roomId)
    if (!room) throw new Error("Room not found")
    return room
  },

  async createRoom(name: string, description: string): Promise<Room> {
    await delay(400)
    const room: Room = {
      id: `room-${Date.now()}`,
      name,
      description,
      owner_id: MOCK_USER_ID,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockRooms = [room, ...mockRooms]
    return room
  },

  async deleteRoom(roomId: string): Promise<void> {
    await delay(300)
    mockRooms = mockRooms.filter((r) => r.id !== roomId)
  },

  async listMessages(roomId: string): Promise<MessagePage> {
    await delay(300)
    const msgs = mockMessages.filter((m) => m.room_id === roomId)
    return { messages: [...msgs].reverse(), next_cursor: null }
  },

  async sendMessage(roomId: string, content: string): Promise<Message> {
    await delay(300)
    const msg: Message = {
      id: `msg-${Date.now()}`,
      room_id: roomId,
      sender_id: MOCK_USER_ID,
      content,
      type: "human",
      status: "completed",
      sequence: nextSeq++,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockMessages.push(msg)
    return msg
  },

  async sendAIMessage(
    roomId: string,
    content: string,
  ): Promise<AIMessageResponse> {
    await delay(300)
    const userMsg: Message = {
      id: `msg-${Date.now()}`,
      room_id: roomId,
      sender_id: MOCK_USER_ID,
      content,
      type: "human",
      status: "completed",
      sequence: nextSeq++,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockMessages.push(userMsg)

    await delay(1000)
    const aiMsg: Message = {
      id: `msg-${Date.now() + 1}`,
      room_id: roomId,
      sender_id: null,
      content:
        "This is a mock AI response. In production, this would connect to the LLM Gateway and return a real AI-generated response based on the conversation context.",
      type: "ai",
      status: "completed",
      sequence: nextSeq++,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockMessages.push(aiMsg)

    return { user_message: userMsg, ai_message: aiMsg }
  },

  async regenerateAIMessage(
    roomId: string,
    messageId: string,
  ): Promise<Message> {
    await delay(1000)
    const updated: Message = {
      id: messageId,
      room_id: roomId,
      sender_id: null,
      content:
        "This is a regenerated mock AI response. The previous response has been replaced with this new one.",
      type: "ai",
      status: "completed",
      sequence: 0,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockMessages = mockMessages.map((m) =>
      m.id === messageId ? { ...updated, sequence: m.sequence } : m,
    )
    return updated
  },
}
