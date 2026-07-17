import { NextResponse, type NextRequest } from "next/server"

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

/** Route handlers must not be statically optimized: every request carries a distinct session cookie. */
export const dynamic = "force-dynamic"

interface RouteContext {
  params: Promise<{ path: string[] }>
}

/**
 * Shared implementation for every HTTP method the data-plane proxy
 * forwards. Rebuilds the upstream URL (path + original query string)
 * against the Go API and forwards the method/body/`Content-Type`/`Cookie`
 * verbatim — no bespoke bearer token is minted or attached here. The
 * browser's `ory_kratos_session` cookie (set on this app's origin by the
 * `/api/kratos/*` proxy after a successful Kratos flow submission) is
 * forwarded as-is; `server/internal/interface/middleware/auth.go` falls
 * back to reading that named cookie when no `Authorization` header is
 * present, so this is sufficient for both public endpoints (e.g.
 * `GET /models`, reachable without a session) and protected endpoints
 * (which get the Go API's own `401` when the cookie is absent or invalid).
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

  const cookie = request.headers.get("Cookie")
  if (cookie) {
    headers["Cookie"] = cookie
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
