"use client"

import { useQuery } from "@tanstack/react-query"
import { getMyInvitationsQueryOptions } from "../api/list-my-invitations"

/**
 * The caller's own pending invitations across every room, sourced from
 * `GET /api/proxy/invitations`. Backs both `InvitationsBellButton`'s badge
 * count and `InvitationsInbox`'s list.
 *
 * Overrides the app-wide `staleTime: 60_000` (see `@/lib/query-client`) with
 * `staleTime: 0` + `refetchOnMount: "always"`: invitations are created by
 * *other* users and no WebSocket event pushes them into this cache, so the
 * badge's initial (often empty) fetch must not suppress a refetch when the
 * `InvitationsInbox` popover actually mounts — otherwise a just-sent
 * invitation stays invisible for up to a minute.
 */
export function useMyInvitations() {
  return useQuery({
    ...getMyInvitationsQueryOptions(),
    staleTime: 0,
    refetchOnMount: "always",
  })
}
