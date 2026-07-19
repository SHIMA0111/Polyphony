import { apiRequest } from "@/lib/http-client"
import type { Room } from "../types"

/**
 * Body for `PATCH /rooms/:roomId/ai-context-cutoff`
 * (`server/internal/interface/handler/dto.go`'s
 * `UpdateRoomAIContextCutoffRequest`). A `null` `cutoff_at` clears the
 * room's AI context cutoff; an RFC3339 timestamp sets it.
 */
export interface UpdateAIContextCutoffInput {
  cutoff_at: string | null
}

/** Calls `PATCH /api/proxy/rooms/:roomId/ai-context-cutoff`. */
export function updateAIContextCutoff(
  roomId: string,
  input: UpdateAIContextCutoffInput,
): Promise<Room> {
  return apiRequest<Room>(`/rooms/${roomId}/ai-context-cutoff`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
}
