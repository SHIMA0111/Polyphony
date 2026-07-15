"use client"

import { useQuery } from "@tanstack/react-query"
import { getMessagesQueryOptions } from "../api/get-messages"

/** Messages in a room, oldest first, sourced from `GET /api/proxy/rooms/:roomId/messages`. */
export function useMessages(roomId: string) {
  return useQuery(getMessagesQueryOptions(roomId))
}
