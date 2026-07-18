/**
 * `sessionStorage` marker recording what kind of Stripe Checkout session the
 * user just started, written immediately before the full-page redirect to
 * Stripe (`useCreateCheckoutSession`'s `onSuccess`) and read back by the
 * post-Checkout success page
 * (`app/(main)/billing/checkout/success/page.tsx`) to decide which resource
 * to poll for the webhook's effect.
 *
 * A subscription purchase changes `["billing", "subscription"]`'s `status`
 * away from `"none"`, so the success page can poll that query directly. A
 * token-pack purchase never touches the subscription — only the balance —
 * so a bare "poll the subscription" strategy left token purchases stuck on
 * a permanent "still processing" state. Instead, the marker snapshots the
 * balance immediately before checkout (`priorBalance`) for that case, and
 * the success page polls `["billing", "balance"]` until it exceeds that
 * snapshot.
 */
export type CheckoutMarker =
  | { kind: "subscription" }
  | { kind: "token_purchase"; priorBalance: number }

/** `sessionStorage` key the marker is stored under. */
const STORAGE_KEY = "polyphony:checkout-marker"

/**
 * Writes {@link CheckoutMarker} to `sessionStorage`, overwriting any
 * previous marker. Called right before the browser navigates away to
 * Stripe Checkout, so it survives the round trip back to the success page
 * (a full page load, which would lose any in-memory/React state).
 */
export function writeCheckoutMarker(marker: CheckoutMarker): void {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(marker))
}

/**
 * Reads and clears the {@link CheckoutMarker} written by
 * {@link writeCheckoutMarker}. Cleared immediately on read (not just on
 * unmount) — this marker is single-use, and leaving it in place would let a
 * later, unrelated visit to the success page (e.g. a bookmarked or
 * back-navigated URL) replay a stale kind/`priorBalance`.
 *
 * @returns `null` if no marker was stored, or the stored value isn't valid
 *   JSON or doesn't match the expected shape (e.g. a tab left open across a
 *   shape change) — callers treat both the same as "no marker", rendering a
 *   neutral success state instead of guessing.
 */
export function readAndClearCheckoutMarker(): CheckoutMarker | null {
  const raw = sessionStorage.getItem(STORAGE_KEY)
  if (raw === null) return null
  sessionStorage.removeItem(STORAGE_KEY)

  try {
    const parsed: unknown = JSON.parse(raw)
    return isCheckoutMarker(parsed) ? parsed : null
  } catch {
    return null
  }
}

/** Runtime shape guard for {@link CheckoutMarker}, used by {@link readAndClearCheckoutMarker}. */
function isCheckoutMarker(value: unknown): value is CheckoutMarker {
  if (typeof value !== "object" || value === null || !("kind" in value)) return false
  const record = value as Record<string, unknown>
  if (record.kind === "subscription") return true
  return record.kind === "token_purchase" && typeof record.priorBalance === "number"
}
