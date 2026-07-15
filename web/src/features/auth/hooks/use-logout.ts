"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { logout } from "../api/logout"

/**
 * Logs the user out, invalidates the cached `["auth", "session"]` query, and
 * navigates to `/login`.
 */
export function useLogout() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] })
      router.push("/login")
    },
  })
}
