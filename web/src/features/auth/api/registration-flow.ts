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
 * Fetches a fresh Kratos self-service registration flow via
 * `GET /api/kratos/self-service/registration/browser`.
 *
 * `Accept: application/json` is set explicitly so Kratos returns the flow
 * JSON directly (its AJAX/SPA contract) instead of a `303` redirect meant
 * for full-page browser navigation.
 *
 * @returns The flow's `ui` container (action URL, method, and field nodes).
 * @throws If the upstream request fails for any reason.
 */
export async function getRegistrationFlow(): Promise<UiContainer> {
  const res = await fetch("/api/kratos/self-service/registration/browser", {
    headers: { Accept: "application/json" },
  })
  if (!res.ok) {
    throw new Error(`Failed to load registration flow: HTTP ${res.status}`)
  }
  const body = (await res.json()) as FlowResponse
  return body.ui
}

/** Field values submitted for the Kratos password registration method. */
export interface RegistrationFlowValues {
  email: string
  username: string
  password: string
  csrf_token: string
}

/**
 * Submits a Kratos registration flow via
 * `POST toRelativeKratosAction(flow.action)`.
 *
 * Posts `{ method: "password", password, traits: { email, username },
 * csrf_token }`, matching the `traits.email`/`traits.username` identity
 * schema (Step 11/Step 20). A `200` response means the account was created
 * and Kratos established the session, resolving `{ ok: true }`. A `400`
 * response body is itself an updated `UiContainer` (e.g. "email already in
 * use"), resolving `{ ok: false, flow: <that body> }`. Any other status is
 * unexpected and throws.
 *
 * @param flow - The previously-fetched flow (only `action` is read here).
 * @param values - The password method's field values, including the flow's
 *   own `csrf_token` echoed back verbatim.
 */
export async function submitRegistrationFlow(
  flow: UiContainer,
  values: RegistrationFlowValues,
): Promise<{ ok: true } | { ok: false; flow: UiContainer }> {
  const { email, username, password, csrf_token } = values

  const res = await fetch(toRelativeKratosAction(flow.action), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({
      method: "password",
      password,
      traits: { email, username },
      csrf_token,
    }),
  })

  if (res.status === 200) {
    return { ok: true }
  }

  if (res.status === 400) {
    const body = (await res.json()) as FlowResponse
    return { ok: false, flow: body.ui }
  }

  throw new Error(`Registration submission failed: HTTP ${res.status}`)
}
