"use client"

import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import { getWsBaseUrl, isMockMode } from "@/config/env"
import { useSession } from "@/features/auth/hooks/use-session"
import { mergeMessageEvent } from "../lib/merge-message-event"
import { isRoomSocketEvent } from "../types/ws-events"
import type { MessagesInfiniteData } from "../lib/message-cache"

/**
 * Live status of a room's WebSocket connection, rendered by
 * `../components/ConnectionStatus.tsx`.
 *
 * `"offline"` covers both an abnormal-close state the reconnect loop has
 * given up reporting more granularly on, and the permanent inert state used
 * in MSW mock mode (see {@link useRoomSocket}'s docstring).
 */
export type ConnectionStatus = "connecting" | "connected" | "reconnecting" | "offline"

/** Response body for `POST /ws/ticket` (Step 15). */
interface WsTicketResponse {
  ticket: string
  expires_in: number
}

/** Initial reconnect delay; doubles on every subsequent failed attempt. */
const BASE_BACKOFF_MS = 500

/** Reconnect delay is never allowed to grow past this. */
const MAX_BACKOFF_MS = 30_000

/** Adds up to 25% random jitter on top of the current backoff delay, so
 * multiple clients reconnecting after the same server blip don't all retry
 * in lockstep. */
function withJitter(backoffMs: number): number {
  return backoffMs + Math.random() * backoffMs * 0.25
}

/**
 * Opens a ticket-authenticated WebSocket connection to the Go API's
 * `GET /rooms/:roomId/ws` endpoint (Step 15) and merges every inbound
 * `message_created`/`message_updated` event into the
 * `["rooms", roomId, "messages"]` TanStack Query cache Step 29 established,
 * via {@link mergeMessageEvent} -- passing along the current user's id
 * (`useSession()`) so that function's defensive sender-mismatch guard for
 * private AI mode (Step 47) has something to compare against.
 *
 * A fresh, short-lived ticket (`POST /ws/ticket`, fetched through
 * `apiRequest` so it goes via the `/api/proxy/*` BFF and is authenticated by
 * the browser's own Kratos session cookie) is minted on every connection
 * attempt — the ticket is never persisted or reused across reconnects,
 * matching the ticket's short (60s) server-side lifetime
 * (`server/internal/interface/wsticket`).
 *
 * The socket itself is opened directly against the Go API's own origin
 * (derived from `NEXT_PUBLIC_API_URL` via `getWsBaseUrl`), not through the
 * Next.js BFF proxy: `/api/proxy/*` cannot upgrade a WebSocket handshake.
 *
 * Reconnects automatically with exponential backoff + jitter (base
 * {@link BASE_BACKOFF_MS}, doubling, capped at {@link MAX_BACKOFF_MS}) after
 * any abnormal close or failed ticket fetch, resetting the backoff delay
 * back to the base value on every successful `onopen`.
 *
 * This WS connection is push-only with no missed-event replay, so any
 * `message_created`/`message_updated` frame published while the connection
 * was down (a dropped wifi connection, a server restart, a laptop sleeping,
 * etc.) is gone for good as far as this socket is concerned. To reconcile
 * that gap, every `onopen` *after* the first one (i.e. one that follows a
 * real disconnect/reconnect cycle, not the initial mount) invalidates
 * `["rooms", roomId, "messages"]` so `useMessages`'s own query refetches the
 * latest page from the REST API. The very first `onopen` does not
 * invalidate -- that would just be a redundant duplicate of the fetch
 * `useMessages` already performs on mount.
 *
 * Fully inert — constructs no `WebSocket` at all and reports a static
 * `"offline"` status — when the app is running under MSW mock mode
 * (`isMockMode()`), since the MSW-mocked REST handlers have no live event
 * source for it to subscribe to.
 *
 * @param roomId - The room to subscribe to. A new connection is opened
 * whenever this changes, tearing down the previous one first.
 * @returns The connection's current {@link ConnectionStatus}.
 */
export function useRoomSocket(roomId: string): ConnectionStatus {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<ConnectionStatus>(
    isMockMode() ? "offline" : "connecting",
  )

  // The current user's id, for `mergeMessageEvent`'s defensive
  // sender-mismatch guard (Step 47). Read via a ref (rather than added to
  // the effect's own dependency array) so a `useSession()` refetch never
  // tears down and reopens the live WebSocket connection below -- only the
  // value `onmessage` reads at event-handling time needs to be current.
  const currentUserId = useSession().data?.identity.id
  const currentUserIdRef = useRef(currentUserId)
  useEffect(() => {
    currentUserIdRef.current = currentUserId
  }, [currentUserId])

  useEffect(() => {
    // The initial `useState` above already covers the mock-mode status; mock
    // mode cannot toggle within a single build (it is a NEXT_PUBLIC_* value
    // inlined at build time), so there is nothing further to synchronize
    // here — just skip opening a real connection.
    if (isMockMode()) return

    const queryKey = ["rooms", roomId, "messages"] as const

    let cancelled = false
    let socket: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    let backoffMs = BASE_BACKOFF_MS
    // Set once the first connection succeeds; gates the reconnect-backfill
    // invalidation below so only a *recovering* onopen (one that follows a
    // real disconnect) triggers it, not the initial mount.
    let hasConnectedOnce = false

    function scheduleReconnect() {
      if (cancelled) return
      setStatus("reconnecting")
      const delay = withJitter(backoffMs)
      backoffMs = Math.min(backoffMs * 2, MAX_BACKOFF_MS)
      reconnectTimer = setTimeout(() => {
        void connect()
      }, delay)
    }

    async function connect() {
      if (cancelled) return
      setStatus((previous) => (previous === "connected" ? previous : "connecting"))

      let ticket: string
      try {
        const response = await apiRequest<WsTicketResponse>("/ws/ticket", {
          method: "POST",
        })
        ticket = response.ticket
      } catch {
        scheduleReconnect()
        return
      }

      if (cancelled) return

      const ws = new WebSocket(
        `${getWsBaseUrl()}/rooms/${roomId}/ws?ticket=${encodeURIComponent(ticket)}`,
      )
      socket = ws

      ws.onopen = () => {
        if (cancelled) return
        backoffMs = BASE_BACKOFF_MS
        setStatus("connected")

        if (hasConnectedOnce) {
          // Recovering from a drop: reconcile whatever was missed while
          // disconnected via a fresh REST fetch (see this hook's docstring).
          void queryClient.invalidateQueries({ queryKey })
        }
        hasConnectedOnce = true
      }

      ws.onmessage = (messageEvent: MessageEvent<string>) => {
        let parsed: unknown
        try {
          parsed = JSON.parse(messageEvent.data)
        } catch {
          return
        }
        if (!isRoomSocketEvent(parsed)) return

        queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
          mergeMessageEvent(old, parsed, currentUserIdRef.current),
        )
      }

      // `onclose` fires for both a clean and an abnormal close (including
      // right after `onerror`, for a handshake that never completed); either
      // way the socket is gone and a reconnect should be scheduled unless
      // this effect has already been cleaned up.
      ws.onclose = () => {
        socket = null
        scheduleReconnect()
      }
    }

    void connect()

    return () => {
      cancelled = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (socket) {
        socket.onopen = null
        socket.onmessage = null
        socket.onclose = null
        socket.onerror = null
        socket.close()
      }
    }
  }, [roomId, queryClient])

  return status
}
