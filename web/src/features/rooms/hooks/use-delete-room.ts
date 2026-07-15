"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { deleteRoom } from "../api/delete-room"

/** Deletes a room and invalidates the `["rooms"]` list query on success. */
export function useDeleteRoom() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: deleteRoom,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
