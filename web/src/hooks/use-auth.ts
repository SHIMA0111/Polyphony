"use client"

import { useState, useEffect, useCallback } from "react"
import { useRouter } from "next/navigation"
import { apiClient } from "@/lib/api"

export function useAuth() {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const router = useRouter()

  useEffect(() => {
    const token = apiClient.getToken()
    setIsAuthenticated(!!token)
    setIsLoading(false)
  }, [])

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await apiClient.login(email, password)
      apiClient.setToken(res.access_token)
      setIsAuthenticated(true)
      router.push("/rooms")
    },
    [router],
  )

  const register = useCallback(
    async (email: string, username: string, password: string) => {
      const res = await apiClient.register(email, username, password)
      apiClient.setToken(res.access_token)
      setIsAuthenticated(true)
      router.push("/rooms")
    },
    [router],
  )

  const logout = useCallback(() => {
    apiClient.clearToken()
    setIsAuthenticated(false)
    router.push("/login")
  }, [router])

  return { isAuthenticated, isLoading, login, register, logout }
}
