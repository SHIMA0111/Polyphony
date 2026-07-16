"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { login } from "../api/login"

/**
 * Logs the user in, invalidates the cached session so `useSession` refetches
 * the now-authenticated `["auth", "session"]` query, and navigates to
 * `/rooms`.
 */
export function useLogin() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: login,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/rooms")
    },
  })
}
