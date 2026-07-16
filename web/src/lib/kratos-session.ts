import "server-only"

import type { KratosSession } from "@/features/auth/types"

/**
 * Base URL of the Kratos public API, read server-side only. Matches the
 * default `app/api/kratos/[...path]/route.ts` falls back to (Step 30) so
 * both modules resolve the same Kratos instance without duplicating a
 * hardcoded value anywhere else.
 */
const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433"

/**
 * Resolves the Kratos session (if any) carried by a raw `Cookie` header.
 *
 * This is the server-only counterpart to
 * `web/src/features/auth/api/get-session.ts`'s client-side `getSession`:
 * both call the exact same Kratos `sessions/whoami` contract, but this
 * helper is invoked from Route Handlers that don't run in a browser
 * request context (`app/(auth)/oauth/{login,consent}/route.ts`), so it
 * takes the incoming request's `Cookie` header directly instead of relying
 * on the browser to attach it automatically.
 *
 * Deliberately a new, standalone module rather than an edit to
 * `app/api/kratos/[...path]/route.ts` or `get-session.ts` (both owned by
 * Step 30) — this step only ever reads Kratos's session state, never
 * Kratos's other self-service flows.
 *
 * @param cookieHeader - The raw `Cookie` header of the incoming request, or
 *   `null` if the request carried none.
 * @returns The parsed Kratos session on `200`, or `null` if Kratos reports
 *   no active session (`401`/`404` — an expired, missing, or forged
 *   cookie).
 * @throws If Kratos responds with any other non-2xx status.
 */
export async function getKratosSessionFromCookie(
  cookieHeader: string | null,
): Promise<KratosSession | null> {
  const res = await fetch(`${KRATOS_PUBLIC_URL}/sessions/whoami`, {
    headers: {
      Accept: "application/json",
      ...(cookieHeader ? { Cookie: cookieHeader } : {}),
    },
    cache: "no-store",
  })

  if (res.status === 401 || res.status === 404) {
    return null
  }

  if (!res.ok) {
    throw new Error(`Failed to fetch Kratos session: HTTP ${res.status}`)
  }

  return (await res.json()) as KratosSession
}
