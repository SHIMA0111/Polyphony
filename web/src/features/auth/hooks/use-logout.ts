"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { logout } from "../api/logout-flow"

/**
 * Logs the user out via Kratos's self-service logout flow, clears the entire
 * React Query cache, and navigates to `/login`.
 *
 * The cache is cleared in full rather than just invalidating the
 * `["auth", "session"]` query: logout ends the session for every user-scoped
 * query (rooms, messages, memberships, ...), and leaving those cached could
 * let a subsequent user on the same device see the previous user's stale
 * data before their own queries refetch.
 *
 * Clearing and navigation run in `onSettled` rather than `onSuccess` so they
 * happen regardless of whether the Kratos logout call itself succeeded or
 * failed: a failed logout request must not strand the user on the current
 * page, since the intent (leave this session) still applies even if the
 * server-side call errored.
 */
export function useLogout() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: logout,
    onSettled: () => {
      queryClient.clear()
      router.push("/login")
    },
  })
}
