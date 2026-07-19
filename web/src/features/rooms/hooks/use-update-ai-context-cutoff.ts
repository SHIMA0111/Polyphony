"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  updateAIContextCutoff,
  type UpdateAIContextCutoffInput,
} from "../api/update-ai-context-cutoff"

/**
 * Sets or clears a room's AI context cutoff
 * (`PATCH /rooms/:roomId/ai-context-cutoff`). Reconciles the
 * `["rooms", roomId]` cache directly with the server's response so
 * `RoomSettingsDrawer` reflects the new cutoff without a full page reload.
 */
export function useUpdateAIContextCutoff(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: UpdateAIContextCutoffInput) =>
      updateAIContextCutoff(roomId, input),
    onSuccess: (data) => {
      queryClient.setQueryData(["rooms", roomId], data)
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
