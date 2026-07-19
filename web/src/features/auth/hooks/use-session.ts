"use client"

import { useQuery } from "@tanstack/react-query"
import { getSessionQueryOptions } from "../api/get-session"

/**
 * Current session, sourced from `GET /api/kratos/sessions/whoami`.
 * `data` is `null` (not undefined+error) once loaded while signed out.
 */
export function useSession() {
  return useQuery(getSessionQueryOptions())
}
