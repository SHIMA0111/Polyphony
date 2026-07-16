"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { register } from "../api/register"

/**
 * Registers a new account, invalidates the cached session so `useSession`
 * refetches the now-authenticated `["auth", "session"]` query, and navigates
 * to `/rooms`.
 */
export function useRegister() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: register,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/rooms")
    },
  })
}
