import { cookies } from "next/headers"
import { NextResponse } from "next/server"

import {
  ACCESS_TOKEN_COOKIE,
  ACCESS_TOKEN_MAX_AGE_SECONDS,
} from "@/lib/auth-cookie"
import type { AuthResponse } from "@/types/api"

/** Base URL of the Go API, read server-side only (never inlined into the client bundle). */
const API_URL = process.env.API_URL ?? "http://localhost:8080"

/**
 * Shared implementation for the `POST /api/auth/login` and
 * `POST /api/auth/register` route handlers.
 *
 * Reads the incoming request body verbatim and forwards it to the given Go
 * API auth endpoint. On a non-OK upstream response, the upstream status and
 * body are returned unchanged so `ApiRequestError`/existing form error
 * handling keeps working. On success, the upstream `TokenResponse`
 * (`{ access_token, token_type }`) is parsed and the access token is stored
 * in an httpOnly session cookie — the raw token is never sent back to the
 * browser in the response body.
 *
 * @param request - The incoming Next.js request (its raw body is forwarded verbatim).
 * @param upstreamPath - The Go API path to forward to (`/auth/login` or `/auth/register`).
 * @returns A `NextResponse` mirroring the upstream error, or `{ ok: true }` on success.
 */
export async function forwardAuthRequest(
  request: Request,
  upstreamPath: "/auth/login" | "/auth/register",
): Promise<NextResponse> {
  const body = await request.text()

  const upstreamRes = await fetch(`${API_URL}${upstreamPath}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
  })

  if (!upstreamRes.ok) {
    const errorBody = await upstreamRes.text()
    return new NextResponse(errorBody, {
      status: upstreamRes.status,
      headers: {
        "Content-Type":
          upstreamRes.headers.get("Content-Type") ?? "application/json",
      },
    })
  }

  const tokenResponse = (await upstreamRes.json()) as AuthResponse

  const cookieStore = await cookies()
  cookieStore.set(ACCESS_TOKEN_COOKIE, tokenResponse.access_token, {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge: ACCESS_TOKEN_MAX_AGE_SECONDS,
  })

  return NextResponse.json({ ok: true })
}
