"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  updateRoomSettings,
  type UpdateRoomSettingsInput,
} from "../api/update-room-settings"

/**
 * Sets or clears a room's default AI provider/model
 * (`PATCH /rooms/:roomId/settings`). Reconciles the `["rooms", roomId]`
 * cache directly with the server's response so `RoomSettingsDrawer`'s model
 * picker re-preselects correctly on the next open, and invalidates
 * `["rooms"]` for consistency with the other room mutations.
 */
export function useUpdateRoomSettings(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: UpdateRoomSettingsInput) =>
      updateRoomSettings(roomId, input),
    onSuccess: (data) => {
      queryClient.setQueryData(["rooms", roomId], data)
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
