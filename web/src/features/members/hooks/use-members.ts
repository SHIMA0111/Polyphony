"use client"

import { useQuery } from "@tanstack/react-query"
import { getMembersQueryOptions } from "../api/get-members"

/** Options accepted by {@link useMembers}. */
interface UseMembersOptions {
  /**
   * Whether this observer should be active. Defaults to `true`.
   *
   * `MemberPanel` passes its own drawer-open state here rather than
   * defaulting to always-on: `refetchOnMount: "always"` only re-fires on an
   * observer's *mount*, and `MemberPanel` (rendered unconditionally by
   * `MemberAvatarStack` so the drawer can animate open/closed) never
   * actually unmounts between opens, so a `refetchOnMount`-only strategy
   * only ever refetches once per room-page visit. Toggling `enabled` from
   * `false` (drawer closed) to `true` (drawer open) makes TanStack Query
   * re-evaluate staleness on that transition — combined with `staleTime: 0`
   * below, every open is guaranteed to issue a fresh request.
   */
  enabled?: boolean
}

/**
 * Members of a room, sourced from `GET /api/proxy/rooms/:roomId/members`.
 *
 * Overrides the app-wide `staleTime: 60_000` (see `@/lib/query-client`) with
 * `staleTime: 0` + `refetchOnMount: "always"`: the member list changes
 * through *other* users' actions (an invitee accepting, another admin
 * changing a role) with no WebSocket event to push those changes into this
 * cache, so `MemberPanel` must refetch every time it opens — otherwise a
 * just-joined member stays invisible for up to a minute. See
 * {@link UseMembersOptions.enabled}'s docstring for why `refetchOnMount`
 * alone isn't sufficient and `enabled` must track the drawer's open state.
 */
export function useMembers(roomId: string, options: UseMembersOptions = {}) {
  return useQuery({
    ...getMembersQueryOptions(roomId),
    staleTime: 0,
    refetchOnMount: "always",
    enabled: options.enabled ?? true,
  })
}
