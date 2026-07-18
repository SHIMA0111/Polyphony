"use client"

import { useQuery } from "@tanstack/react-query"
import { getInvitationByCodeQueryOptions } from "../api/get-invitation-by-code"

/**
 * A single invitation, previewed by its shareable `invite_code`, sourced
 * from `GET /api/proxy/invitations/by-code/:code`. Backs the `/invite/[code]`
 * landing page.
 */
export function useInvitationByCode(code: string) {
  return useQuery(getInvitationByCodeQueryOptions(code))
}
