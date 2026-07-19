import { apiRequest } from "@/lib/http-client"
import type { BillingPortalSessionResponse } from "../types"

/** Poster shape shared by mutation `api/` modules that only need `POST`. */
type Poster = <T>(path: string, options: RequestInit) => Promise<T>

/**
 * Calls `POST /api/proxy/billing/portal-session` with the current page's
 * origin as the Stripe billing portal's `return_url`, per Step 49's
 * `BillingPortalRequest` — Stripe redirects back to `/billing/subscription`
 * once the user is done in the hosted portal.
 */
export function createBillingPortalSession(
  poster: Poster = apiRequest,
): Promise<BillingPortalSessionResponse> {
  return poster<BillingPortalSessionResponse>("/billing/portal-session", {
    method: "POST",
    body: JSON.stringify({
      return_url: `${window.location.origin}/billing/subscription`,
    }),
  })
}
