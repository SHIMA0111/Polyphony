import { cookies } from "next/headers"
import { NextResponse } from "next/server"

import { ACCESS_TOKEN_COOKIE } from "@/lib/auth-cookie"

/**
 * `POST /api/auth/logout` — clears the httpOnly `access_token` cookie.
 *
 * No Go API call is needed: `server/internal/interface/handler/auth_handler.go`
 * has no logout endpoint, and SimpleJWT tokens are stateless, so clearing the
 * cookie is sufficient to end the browser's session.
 */
export async function POST() {
  const cookieStore = await cookies()
  cookieStore.delete(ACCESS_TOKEN_COOKIE)
  return NextResponse.json({ ok: true })
}
