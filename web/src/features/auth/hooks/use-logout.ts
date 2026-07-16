"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { logout } from "../api/logout-flow"

/**
 * Logs the user out via Kratos's self-service logout flow, invalidates the
 * cached `["auth", "session"]` query, and navigates to `/login`.
 *
 * Invalidation and navigation run in `onSettled` rather than `onSuccess` so
 * they happen regardless of whether the Kratos logout call itself succeeded
 * or failed: a failed logout request must not strand the user on the
 * current page, since the intent (leave this session) still applies even
 * if the server-side call errored.
 */
export function useLogout() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: logout,
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/login")
    },
  })
}
