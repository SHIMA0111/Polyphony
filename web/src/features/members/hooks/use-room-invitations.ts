"use client"

import { useQuery } from "@tanstack/react-query"
import { getRoomInvitationsQueryOptions } from "../api/list-room-invitations"

/** All invitations for a room, sourced from `GET /api/proxy/rooms/:roomId/invitations`. */
export function useRoomInvitations(roomId: string) {
  return useQuery(getRoomInvitationsQueryOptions(roomId))
}
