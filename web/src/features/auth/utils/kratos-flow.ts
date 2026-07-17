/**
 * Shared shapes and helpers for Kratos's self-service flow `ui` payload
 * (the `ui` object embedded in every login/registration/settings flow
 * response), used by `LoginForm`/`RegisterForm` and the `api/*-flow.ts`
 * modules that fetch/submit those flows through `/api/kratos/*`.
 *
 * These types intentionally model only the subset of Kratos's flow schema
 * this app renders/consumes — see Kratos's own OpenAPI spec for the full
 * shape (which also includes flow-level fields like `id`/`expires_at`/
 * `state` that this app never needs client-side).
 */

/** A single Kratos self-service flow-level or field-level message. */
export interface UiNodeMessage {
  id: number
  text: string
  type: "error" | "info" | "success"
}

/**
 * A single form field (or hidden field, or informational node) Kratos asks
 * the UI to render as part of a self-service flow.
 *
 * `attributes.name` is the field name to submit back (e.g. `"identifier"`,
 * `"password"`, `"csrf_token"`, `"traits.email"`, `"traits.username"`).
 */
export interface UiNode {
  type: "input" | "text" | "img" | "script"
  group: string
  attributes: {
    name?: string
    type?: string
    value?: unknown
    required?: boolean
    disabled?: boolean
  }
  messages: UiNodeMessage[]
}

/**
 * The `ui` object embedded in every Kratos self-service flow response
 * (`GET .../browser`) and in the `400` validation-failure body returned
 * from submitting one.
 */
export interface UiContainer {
  /**
   * Absolute URL (against Kratos's own `serve.public.base_url`) to submit
   * the flow to. Always pass this through {@link toRelativeKratosAction}
   * before using it in a `fetch` call — the browser can never resolve
   * Kratos's own hostname directly.
   */
  action: string
  method: string
  nodes: UiNode[]
  messages?: UiNodeMessage[]
}

/**
 * Rewrites an absolute Kratos `ui.action` (or `logout_url`) into a
 * same-origin path through this app's `/api/kratos/*` proxy.
 *
 * Kratos returns `ui.action`/`logout_url` as an absolute URL against its own
 * configured `serve.public.base_url` (e.g. `http://localhost:4433/...` or
 * `http://kratos:4433/...` depending on environment) — the browser can
 * never resolve `kratos:4433` at all, and even `localhost:4433` would be a
 * cross-origin request that wouldn't carry this app's cookies the way a
 * same-origin `/api/kratos/*` call does. This strips whatever origin is
 * present and rewrites the path so every submission stays same-origin.
 *
 * @param action - The absolute (or already-relative) URL Kratos returned.
 * @returns A same-origin path beginning with `/api/kratos`.
 */
export function toRelativeKratosAction(action: string): string {
  // `URL` requires a base for relative inputs; a dummy base is discarded
  // once we only read `pathname`/`search`, so this works whether `action`
  // is absolute or already relative.
  const url = new URL(action, "http://placeholder.invalid")
  return `/api/kratos${url.pathname}${url.search}`
}

/**
 * Reads the current value of a named node's `attributes.value` out of a
 * flow's `nodes[]` — used for hidden fields like `csrf_token` whose value
 * must be echoed back verbatim on submission.
 *
 * @param nodes - The flow's `ui.nodes` array.
 * @param name - The node's `attributes.name` to look up.
 * @returns The node's string value, or `""` if the node is absent or its
 *   value isn't a string.
 */
export function getNodeValue(nodes: UiNode[], name: string): string {
  const node = nodes.find((n) => n.attributes.name === name)
  const value = node?.attributes.value
  return typeof value === "string" ? value : ""
}

/**
 * Collects the `text` of every message attached to a named node — used to
 * render field-level validation/error text (e.g. "invalid characters")
 * inline under that field.
 *
 * @param nodes - The flow's `ui.nodes` array.
 * @param name - The node's `attributes.name` to look up.
 * @returns The node's message texts, or `[]` if the node has none.
 */
export function getNodeMessages(nodes: UiNode[], name: string): string[] {
  const node = nodes.find((n) => n.attributes.name === name)
  return node?.messages.map((m) => m.text) ?? []
}

/**
 * Collects the text of every flow-level (non-field) message — e.g. "The
 * provided credentials are invalid" — for display via the toaster rather
 * than under any specific field.
 *
 * @param ui - The flow's `ui` container.
 * @returns The flow-level message texts, or `[]` if there are none.
 */
export function getFormMessages(ui: UiContainer): string[] {
  return ui.messages?.map((m) => m.text) ?? []
}
