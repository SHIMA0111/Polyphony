/**
 * `sessionStorage` marker recording what kind of Stripe Checkout session was
 * just started, so `app/(main)/billing/checkout/success/page.tsx` knows
 * whether to poll the subscription query or the balance query once Stripe
 * redirects back here.
 *
 * This can't be in-memory React state: starting Checkout is a full
 * `window.location.href` browser navigation away from the app (see
 * `use-create-checkout-session.ts`), so nothing survives except storage that
 * outlives the page. A one-time token-pack purchase never changes the
 * subscription, so without this marker the success page would poll
 * `useSubscription()` for a token purchase and never resolve.
 *
 * Written immediately before the redirect and consumed exactly once (via
 * {@link readAndClearCheckoutMarker}) by the success page, which clears it
 * so that revisiting `/billing/checkout/success` directly (e.g. via the
 * back button, or a bookmarked link) falls back to the neutral
 * marker-missing state instead of replaying a stale poll.
 */

/** The `sessionStorage` key this module owns. */
const STORAGE_KEY = "billing:checkout-marker"

/**
 * What the success page should poll for. `"token_purchase"` carries the
 * balance observed at checkout-initiation time so the success page can
 * detect the credit by watching for the balance to exceed it, rather than
 * polling a subscription that a token pack never changes.
 */
export type CheckoutMarker =
  | { kind: "subscription" }
  | { kind: "token_purchase"; priorBalance: number }

/**
 * Persists `marker` for the success page to pick up after Stripe's hosted
 * Checkout redirects back. Swallows `sessionStorage` write failures (private
 * browsing / storage disabled) — the success page's marker-missing fallback
 * already handles that case as a neutral, non-erroring state.
 */
export function writeCheckoutMarker(marker: CheckoutMarker): void {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(marker))
  } catch {
    // Best-effort only; see doc comment above.
  }
}

/**
 * Reads and removes the marker written by {@link writeCheckoutMarker}.
 * Returns `null` if no marker was ever written, it failed to parse, or
 * `sessionStorage` is unavailable — every one of those is treated as the
 * same "marker missing" case by the success page.
 */
export function readAndClearCheckoutMarker(): CheckoutMarker | null {
  try {
    const raw = window.sessionStorage.getItem(STORAGE_KEY)
    window.sessionStorage.removeItem(STORAGE_KEY)
    if (!raw) return null

    const parsed = JSON.parse(raw) as Partial<CheckoutMarker> | null
    if (parsed?.kind === "subscription") {
      return { kind: "subscription" }
    }
    if (parsed?.kind === "token_purchase" && typeof parsed.priorBalance === "number") {
      return { kind: "token_purchase", priorBalance: parsed.priorBalance }
    }
    return null
  } catch {
    return null
  }
}
