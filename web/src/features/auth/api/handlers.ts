import { http, HttpResponse } from "msw"
import type { KratosSession } from "../types"
import type { UiContainer } from "../utils/kratos-flow"

/**
 * MSW request handlers for the auth feature, shared by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`) and the browser
 * `setupWorker` (`src/test/msw/browser.ts`).
 *
 * Mocks the `/api/kratos/*` proxy's Kratos-shaped responses (self-service
 * login/registration/logout flows and `sessions/whoami`) rather than the
 * retired `/api/auth/*` JWT endpoints — this app now speaks Kratos's flow
 * contract end to end, even in tests. The exported handlers below default
 * to the "happy path" (flow loads, submission succeeds); individual tests
 * override the `POST` submission handlers via `server.use(...)` with
 * {@link makeLoginFlowUiWithError}/{@link makeRegistrationFlowUiWithError}
 * to exercise the `400` validation-failure path.
 */

/** Canned CSRF token echoed back on every mocked flow, matching what a real Kratos flow embeds. */
const CSRF_TOKEN = "test-csrf-token"

/** The fixture user's session, returned by the mocked `sessions/whoami` handler. */
export const fixtureSession: KratosSession = {
  identity: {
    id: "user-1",
    traits: {
      email: "user@example.com",
      username: "testuser",
    },
  },
}

const LOGIN_FLOW_ACTION = "http://localhost:4433/self-service/login?flow=login-flow-id"

/** A fresh (no errors) login flow `ui` container. */
export function makeLoginFlowUi(): UiContainer {
  return {
    action: LOGIN_FLOW_ACTION,
    method: "POST",
    nodes: [
      {
        type: "input",
        group: "default",
        attributes: { name: "csrf_token", type: "hidden", value: CSRF_TOKEN, required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "identifier", type: "text", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "password", type: "password", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "method", type: "submit", value: "password" },
        messages: [],
      },
    ],
  }
}

/** The same login flow, re-rendered with a credentials-invalid error (Kratos's `400` shape). */
export function makeLoginFlowUiWithError(): UiContainer {
  const ui = makeLoginFlowUi()
  ui.messages = [
    {
      id: 4000006,
      text: "The provided credentials are invalid, check for spelling mistakes in your password or username, email address, or phone number.",
      type: "error",
    },
  ]
  return ui
}

const REGISTRATION_FLOW_ACTION =
  "http://localhost:4433/self-service/registration?flow=registration-flow-id"

/** A fresh (no errors) registration flow `ui` container. */
export function makeRegistrationFlowUi(): UiContainer {
  return {
    action: REGISTRATION_FLOW_ACTION,
    method: "POST",
    nodes: [
      {
        type: "input",
        group: "default",
        attributes: { name: "csrf_token", type: "hidden", value: CSRF_TOKEN, required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "traits.email", type: "text", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "traits.username", type: "text", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "password", type: "password", required: true },
        messages: [],
      },
      {
        type: "input",
        group: "password",
        attributes: { name: "method", type: "submit", value: "password" },
        messages: [],
      },
    ],
  }
}

/** The same registration flow, re-rendered with an "email already in use" error (Kratos's `400` shape). */
export function makeRegistrationFlowUiWithError(): UiContainer {
  const ui = makeRegistrationFlowUi()
  const emailNode = ui.nodes.find((n) => n.attributes.name === "traits.email")
  const message = {
    id: 4000007,
    text: "An account with the same identifier (email) exists already.",
    type: "error" as const,
  }
  if (emailNode) {
    emailNode.messages = [message]
  }
  ui.messages = [message]
  return ui
}

export const authHandlers = [
  http.get("/api/kratos/self-service/login/browser", () => {
    return HttpResponse.json({ ui: makeLoginFlowUi() })
  }),

  http.post("/api/kratos/self-service/login", () => {
    return HttpResponse.json({})
  }),

  http.get("/api/kratos/self-service/registration/browser", () => {
    return HttpResponse.json({ ui: makeRegistrationFlowUi() })
  }),

  http.post("/api/kratos/self-service/registration", () => {
    return HttpResponse.json({})
  }),

  http.get("/api/kratos/self-service/logout/browser", () => {
    return HttpResponse.json({
      logout_url: "http://localhost:4433/self-service/logout?token=test-logout-token",
    })
  }),

  http.get("/api/kratos/self-service/logout", () => {
    return new HttpResponse(null, { status: 200 })
  }),

  http.get("/api/kratos/sessions/whoami", () => {
    return HttpResponse.json<KratosSession>(fixtureSession)
  }),
]
