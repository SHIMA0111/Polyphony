"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { createRoom } from "../api/create-room"

/** Creates a room and invalidates the `["rooms"]` list query on success. */
export function useCreateRoom() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: createRoom,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
