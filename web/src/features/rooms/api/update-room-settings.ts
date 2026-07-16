import { apiRequest } from "@/lib/http-client"
import type { Room } from "../types"

/**
 * Body for `PATCH /rooms/:roomId/settings`
 * (`server/internal/usecase/room/settings.go`'s `UpdateSettings`). Both
 * fields always follow the same "value vs. empty-string" convention: a
 * concrete `ai_provider`/`ai_model` pins the room to that provider+model, an
 * empty string clears the room back to the deployment-wide default. This
 * client never omits either field, so the server's third "leave unchanged"
 * (field-omitted) convention is not exercised here.
 */
export interface UpdateRoomSettingsInput {
  ai_provider: string
  ai_model: string
}

/** Calls `PATCH /api/proxy/rooms/:roomId/settings`. */
export function updateRoomSettings(
  roomId: string,
  input: UpdateRoomSettingsInput,
): Promise<Room> {
  return apiRequest<Room>(`/rooms/${roomId}/settings`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
}
