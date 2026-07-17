import { toRelativeKratosAction, type UiContainer } from "../utils/kratos-flow"

/**
 * Shape of the top-level Kratos flow response — only the `ui` sub-object is
 * consumed by this app, but Kratos's actual payload also carries `id`,
 * `expires_at`, etc., which this app never needs.
 */
interface FlowResponse {
  ui: UiContainer
}

/**
 * Fetches a fresh Kratos self-service login flow via
 * `GET /api/kratos/self-service/login/browser`.
 *
 * `Accept: application/json` is set explicitly so Kratos returns the flow
 * JSON directly (its AJAX/SPA contract) instead of a `303` redirect meant
 * for full-page browser navigation.
 *
 * @returns The flow's `ui` container (action URL, method, and field nodes).
 * @throws If the upstream request fails for any reason other than the flow
 *   itself being returned (Kratos only ever `200`s this endpoint).
 */
export async function getLoginFlow(): Promise<UiContainer> {
  const res = await fetch("/api/kratos/self-service/login/browser", {
    headers: { Accept: "application/json" },
  })
  if (!res.ok) {
    throw new Error(`Failed to load login flow: HTTP ${res.status}`)
  }
  const body = (await res.json()) as FlowResponse
  return body.ui
}

/** Field values submitted for the Kratos password login method. */
export interface LoginFlowValues {
  identifier: string
  password: string
  csrf_token: string
}

/**
 * Submits a Kratos login flow via `POST toRelativeKratosAction(flow.action)`.
 *
 * A `200` response means Kratos has established the session (its
 * `Set-Cookie` already round-tripped through `/api/kratos/*`), resolving
 * `{ ok: true }`. A `400` response body is itself an updated `UiContainer`
 * (Kratos re-renders the flow with validation/credential-error messages
 * attached to its nodes), resolving `{ ok: false, flow: <that body> }` so
 * the caller can re-render with the fresh flow. Any other status is
 * unexpected and throws.
 *
 * @param flow - The previously-fetched flow (only `action` is read here).
 * @param values - The password method's field values, including the flow's
 *   own `csrf_token` echoed back verbatim.
 */
export async function submitLoginFlow(
  flow: UiContainer,
  values: LoginFlowValues,
): Promise<{ ok: true } | { ok: false; flow: UiContainer }> {
  const res = await fetch(toRelativeKratosAction(flow.action), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ method: "password", ...values }),
  })

  if (res.status === 200) {
    return { ok: true }
  }

  if (res.status === 400) {
    const body = (await res.json()) as FlowResponse
    return { ok: false, flow: body.ui }
  }

  throw new Error(`Login submission failed: HTTP ${res.status}`)
}
