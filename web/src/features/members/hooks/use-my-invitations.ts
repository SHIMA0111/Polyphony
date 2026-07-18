"use client"

import { useQuery } from "@tanstack/react-query"
import { getMyInvitationsQueryOptions } from "../api/list-my-invitations"

/**
 * The caller's own pending invitations across every room, sourced from
 * `GET /api/proxy/invitations`. Backs both `InvitationsBellButton`'s badge
 * count and `InvitationsInbox`'s list.
 */
export function useMyInvitations() {
  return useQuery(getMyInvitationsQueryOptions())
}
