"use client"

import { useEffect, useState } from "react"
import type { Room } from "@/types/api"

/**
 * Minimal example hook fetching the room list from the `/api/proxy/rooms` BFF
 * proxy endpoint (see Step 4's `app/api/proxy/[...path]/route.ts`).
 *
 * This hook exists solely to prove the `/api/proxy/*` MSW contract end-to-end
 * from a React hook (see `use-rooms.test.ts`) as part of this repo's Vitest +
 * MSW test harness. It is a deliberately small, temporary placeholder,
 * distinct from `web/src/lib/api.ts`'s `apiClient` — it does not handle
 * auth headers, retries, or caching.
 *
 * Step 9 (web data layer migration) replaces this hook with a TanStack Query
 * hook backed by a per-feature `rooms/api.ts` module, reusing the same MSW
 * handlers unchanged.
 *
 * @returns An object with the current `rooms` list, `isLoading` flag, and
 *   `error` message (`null` when no error has occurred).
 */
export function useRooms(): {
  rooms: Room[]
  isLoading: boolean
  error: string | null
} {
  const [rooms, setRooms] = useState<Room[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function fetchRooms() {
      setIsLoading(true)
      setError(null)
      try {
        const res = await fetch("/api/proxy/rooms")
        if (!res.ok) {
          throw new Error(`Request failed: ${res.status}`)
        }
        const data = (await res.json()) as Room[]
        if (!cancelled) {
          setRooms(data)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Unknown error")
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    void fetchRooms()

    return () => {
      cancelled = true
    }
  }, [])

  return { rooms, isLoading, error }
}
