import { NextResponse, type NextRequest } from "next/server"

import { KRATOS_SESSION_COOKIE_NAME } from "@/lib/kratos-cookie"

const AUTH_PATHS = ["/login", "/register"]
const PROTECTED_PATHS = ["/rooms", "/invite"]

/**
 * Matches `pathname` against `path` on a segment boundary (`pathname ===
 * path`, or `pathname` starts with `path` followed by `/`) rather than a
 * bare `startsWith`, which would also match an unrelated route that merely
 * shares `path`'s prefix (e.g. `/roomsxyz` for `/rooms`, `/invitee` for
 * `/invite`).
 */
function matchesPathBoundary(pathname: string, path: string): boolean {
  return pathname === path || pathname.startsWith(`${path}/`)
}

/**
 * Route guard for the `(main)` and `(auth)` App Router groups.
 *
 * This is a UX fast-path only, NOT the security boundary: it merely checks
 * whether the `ory_kratos_session` cookie is present and redirects
 * accordingly so logged-out users don't briefly see protected pages (and
 * vice versa). The actual authorization decision is still made by the Go
 * API — every proxied request goes through
 * `server/internal/interface/middleware/auth.go`'s auth middleware, which
 * resolves the session against Kratos on every request. A forged or stale
 * cookie would pass this check but still be rejected by the Go API with a
 * `401`.
 *
 * Bypassed entirely in mock mode (`NEXT_PUBLIC_MOCK_API=true`) so the
 * existing no-login demo flow keeps working.
 */
export function middleware(request: NextRequest) {
  if (process.env.NEXT_PUBLIC_MOCK_API === "true") {
    return NextResponse.next()
  }

  const hasSession = request.cookies.has(KRATOS_SESSION_COOKIE_NAME)
  const { pathname } = request.nextUrl

  const isAuthPath = AUTH_PATHS.some((path) => matchesPathBoundary(pathname, path))
  const isProtectedPath = PROTECTED_PATHS.some((path) => matchesPathBoundary(pathname, path))

  if (!hasSession && isProtectedPath) {
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
