import { NextResponse, type NextRequest } from "next/server"

import { ACCESS_TOKEN_COOKIE } from "@/lib/auth-cookie"

const AUTH_PATHS = ["/login", "/register"]

/**
 * Route guard for the `(main)` and `(auth)` App Router groups.
 *
 * This is a UX fast-path only, NOT the security boundary: it merely checks
 * whether the `access_token` cookie is present and redirects accordingly so
 * logged-out users don't briefly see protected pages (and vice versa). The
 * actual authorization decision is still made by the Go API — every proxied
 * request goes through `server/internal/interface/middleware/auth.go`'s
 * `JWTAuth`, which validates the token's signature and expiry on every
 * request. A forged or stale cookie would pass this check but still be
 * rejected by the Go API with a `401`.
 *
 * Bypassed entirely in mock mode (`NEXT_PUBLIC_MOCK_API=true`) so the
 * existing no-login demo flow keeps working.
 */
export function middleware(request: NextRequest) {
  if (process.env.NEXT_PUBLIC_MOCK_API === "true") {
    return NextResponse.next()
  }

  const hasSession = request.cookies.has(ACCESS_TOKEN_COOKIE)
  const { pathname } = request.nextUrl

  const isAuthPath = AUTH_PATHS.some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  )

  if (!hasSession && pathname.startsWith("/rooms")) {
    return NextResponse.redirect(new URL("/login", request.url))
  }

  if (hasSession && isAuthPath) {
    return NextResponse.redirect(new URL("/rooms", request.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: ["/((?!api|_next|favicon.ico).*)"],
}
