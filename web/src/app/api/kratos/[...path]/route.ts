import { NextResponse, type NextRequest } from "next/server"

/** Base URL of the Kratos public API, read server-side only (never inlined into the client bundle). */
const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433"

/** Route handlers must not be statically optimized: every request carries a distinct session cookie. */
export const dynamic = "force-dynamic"

interface RouteContext {
  params: Promise<{ path: string[] }>
}

/**
 * Shared implementation for the `GET`/`POST` methods this catch-all proxy
 * forwards to Kratos's public API (self-service login/registration/logout
 * flows and `sessions/whoami`).
 *
 * Forwards the incoming request's raw `Cookie` header verbatim (the Kratos
 * session cookie, and the flow's CSRF cookie set by the preceding `GET
 * .../browser` call, both round-trip through this same-origin proxy so the
 * browser attaches them automatically) and the request body verbatim on
 * `POST`. Sets `Accept: application/json` on the upstream request — Kratos's
 * AJAX/SPA contract: without it, `self-service/{login,registration}/browser`
 * and flow submission endpoints respond with `302`/`303` HTML redirects
 * intended for full-page browser navigation instead of returning the
 * flow/session JSON this proxy's callers expect.
 *
 * Every upstream `Set-Cookie` header is copied onto the outgoing
 * `NextResponse` — Kratos may set more than one (e.g. session + CSRF
 * cookies), and `Headers.get("set-cookie")` only ever returns one, so
 * `getSetCookie()` is used and each value is appended individually.
 *
 * The upstream body is streamed back unchanged, but only `Content-Type` is
 * copied from the upstream response's other headers — `Content-Encoding`/
 * `Transfer-Encoding` are intentionally dropped since the body was already
 * read and decoded here (mirrors `app/api/proxy/[...path]/route.ts`).
 *
 * @param request - The incoming Next.js request.
 * @param context - Route context carrying the (Next 16 async) dynamic `path` segments.
 * @returns A `NextResponse` mirroring the upstream status, body, and `Set-Cookie` headers.
 */
async function proxy(
  request: NextRequest,
  context: RouteContext,
): Promise<NextResponse> {
  const { path } = await context.params
  const upstreamUrl = `${KRATOS_PUBLIC_URL}/${path.join("/")}${request.nextUrl.search}`

  const headers: Record<string, string> = {
    Accept: "application/json",
  }
  const contentType = request.headers.get("Content-Type")
  if (contentType) {
    headers["Content-Type"] = contentType
  }
  const cookie = request.headers.get("Cookie")
  if (cookie) {
    headers["Cookie"] = cookie
  }

  const hasBody = request.method !== "GET" && request.method !== "HEAD"

  const upstreamRes = await fetch(upstreamUrl, {
    method: request.method,
    headers,
    body: hasBody ? await request.arrayBuffer() : undefined,
  })

  const responseBody = await upstreamRes.arrayBuffer()
  const responseHeaders = new Headers()
  const upstreamContentType = upstreamRes.headers.get("Content-Type")
  if (upstreamContentType) {
    responseHeaders.set("Content-Type", upstreamContentType)
  }
  for (const cookieValue of upstreamRes.headers.getSetCookie()) {
    responseHeaders.append("Set-Cookie", cookieValue)
  }

  return new NextResponse(responseBody, {
    status: upstreamRes.status,
    headers: responseHeaders,
  })
}

export async function GET(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}

export async function POST(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}
