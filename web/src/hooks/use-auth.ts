"use client"

import { useCallback } from "react"
import { useRouter } from "next/navigation"
import { apiClient } from "@/lib/api"

/**
 * Thin async wrappers around `apiClient`'s auth methods that also handle
 * post-auth navigation.
 *
 * The JWT now lives server-side in an httpOnly cookie (set/cleared by the
 * `/api/auth/*` route handlers), so there is no client-visible token to
 * derive an `isAuthenticated`/`isLoading` state from — route protection is
 * handled by `middleware.ts` instead.
 */
export function useAuth() {
  const router = useRouter()

  const login = useCallback(
    async (email: string, password: string) => {
      await apiClient.login(email, password)
      router.push("/rooms")
    },
    [router],
  )

  const register = useCallback(
    async (email: string, username: string, password: string) => {
      await apiClient.register(email, username, password)
      router.push("/rooms")
    },
    [router],
  )

  const logout = useCallback(async () => {
    await apiClient.logout()
    router.push("/login")
  }, [router])

  return { login, register, logout }
}
