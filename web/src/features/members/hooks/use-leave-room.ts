"use client"

import { useRouter } from "next/navigation"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { leaveRoom } from "../api/leave-room"

/**
 * Leaves a room the caller doesn't own (self-leave; the server 409s if the
 * caller currently owns the room). On success, invalidates the `["rooms"]`
 * list (the left room no longer belongs to the caller) and navigates back to
 * `/rooms`, since the room view the caller was just in is no longer
 * accessible to them.
 */
export function useLeaveRoom(roomId: string, userId: string) {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: () => leaveRoom(roomId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
      router.push("/rooms")
    },
  })
}
