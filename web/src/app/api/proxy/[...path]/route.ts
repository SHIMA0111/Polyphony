import { NextResponse, type NextRequest } from "next/server"

/** Base URL of the Go API, read server-side only (never inlined into the client bundle). */
const API_URL = process.env.API_URL ?? "http://localhost:8080"

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
 * The upstream body is fully buffered via `arrayBuffer()` before
 * `NextResponse` is constructed, not streamed back as it arrives. Only
 * `Content-Type` and `Retry-After` are forwarded from the upstream
 * response — `Content-Encoding`/`Transfer-Encoding` are intentionally
 * dropped since the body was already read and decoded here. `Retry-After`
 * is forwarded (Step 57's `rate-limiting.spec.ts` regression coverage)
 * because `middleware.RateLimit` (Step 33) sets it on every HTTP 429
 * response and a caller cannot compute a sane backoff without it —
 * omitting it silently turned every rate-limit-aware client into one that
 * can't actually back off correctly.
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

  const upstreamRes = await fetch(upstreamUrl, {
    method: request.method,
    headers,
    body: hasBody ? await request.arrayBuffer() : undefined,
  })

  const responseBody = await upstreamRes.arrayBuffer()
  const responseHeaders: Record<string, string> = {}
  const upstreamContentType = upstreamRes.headers.get("Content-Type")
  if (upstreamContentType) {
    responseHeaders["Content-Type"] = upstreamContentType
  }
  const upstreamRetryAfter = upstreamRes.headers.get("Retry-After")
  if (upstreamRetryAfter) {
    responseHeaders["Retry-After"] = upstreamRetryAfter
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
