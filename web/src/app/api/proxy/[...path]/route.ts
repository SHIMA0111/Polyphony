import { cookies } from "next/headers"
import { NextResponse, type NextRequest } from "next/server"

import { ACCESS_TOKEN_COOKIE } from "@/lib/auth-cookie"

/** Base URL of the Go API, read server-side only (never inlined into the client bundle). */
const API_URL = process.env.API_URL ?? "http://localhost:8080"

/** Default upstream request timeout in milliseconds, overridable via `PROXY_UPSTREAM_TIMEOUT_MS`. */
const DEFAULT_UPSTREAM_TIMEOUT_MS = 30_000

/** Upstream request timeout in milliseconds, read from `PROXY_UPSTREAM_TIMEOUT_MS` (default 30s). */
const UPSTREAM_TIMEOUT_MS = (() => {
  const raw = process.env.PROXY_UPSTREAM_TIMEOUT_MS
  const parsed = raw ? Number(raw) : NaN
  return Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_UPSTREAM_TIMEOUT_MS
})()

/** Route handlers must not be statically optimized: every request reads the session cookie. */
export const dynamic = "force-dynamic"

interface RouteContext {
  params: Promise<{ path: string[] }>
}

/**
 * Shared implementation for every HTTP method the data-plane proxy
 * forwards. Reads the `access_token` cookie server-side, rebuilds the
 * upstream URL (path + original query string) against the Go API, forwards
 * the method/body/`Content-Type`, and attaches `Authorization: Bearer
 * <token>` only when the cookie is present — public endpoints (e.g.
 * `GET /models`) still work without a session, and protected endpoints get
 * the Go API's own `401` when the cookie is absent.
 *
 * The upstream body is streamed back unchanged, but the only response
 * header forwarded is `Content-Type` — `Content-Encoding`/
 * `Transfer-Encoding` are intentionally dropped since the body was already
 * read and decoded here.
 *
 * @param request - The incoming Next.js request.
 * @param context - Route context carrying the (Next 16 async) dynamic `path` segments.
 * @returns A `NextResponse` mirroring the upstream status and body.
 */
async function proxy(
  request: NextRequest,
  context: RouteContext,
): Promise<NextResponse> {
  const { path } = await context.params
  const upstreamUrl = `${API_URL}/${path.join("/")}${request.nextUrl.search}`

  const headers: Record<string, string> = {}
  const contentType = request.headers.get("Content-Type")
  if (contentType) {
    headers["Content-Type"] = contentType
  }

  const cookieStore = await cookies()
  const token = cookieStore.get(ACCESS_TOKEN_COOKIE)?.value
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const hasBody = request.method !== "GET" && request.method !== "HEAD"

  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), UPSTREAM_TIMEOUT_MS)

  let upstreamRes: Response
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      signal: controller.signal,
    })
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") {
      return NextResponse.json({ message: "upstream timeout" }, { status: 504 })
    }
    return NextResponse.json({ message: "upstream unreachable" }, { status: 502 })
  } finally {
    clearTimeout(timeout)
  }

  const responseBody = await upstreamRes.arrayBuffer()
  const responseHeaders: Record<string, string> = {}
  const upstreamContentType = upstreamRes.headers.get("Content-Type")
  if (upstreamContentType) {
    responseHeaders["Content-Type"] = upstreamContentType
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

export async function PUT(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}

export async function PATCH(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}

export async function DELETE(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}
