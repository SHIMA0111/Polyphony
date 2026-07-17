"use client"

import { useQuery } from "@tanstack/react-query"
import { getCurrentUserQueryOptions } from "../api/get-current-user"

/**
 * The signed-in viewer's own local user record (`GET /users/me`).
 *
 * Use `data?.id` (not `useSession().data?.identity.id`) for any "is this the
 * viewer's own X" comparison against a `sender_id`/`user_id` elsewhere in the
 * app -- see `CurrentUser`'s doc comment in `../types.ts` for why the two
 * ids can never match under `AUTH_MODE=kratos`.
 */
export function useCurrentUser() {
  return useQuery(getCurrentUserQueryOptions())
}
