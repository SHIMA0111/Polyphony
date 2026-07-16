/**
 * Extracts a human-readable message from a caught error, falling back to a
 * caller-supplied default for a non-`Error` throw (M5 post-review dedup: this
 * exact `err instanceof Error ? err.message : "..."` ternary was previously
 * duplicated across roughly twenty toast/inline-error call sites).
 *
 * `ApiRequestError` (`@/lib/http-client`) extends `Error` and always calls
 * `super(message)`, so it is already covered by the `instanceof Error`
 * branch -- no special-casing is needed to get its normalized `.message`.
 *
 * @param err - The caught value (typed `unknown`, matching a `catch` clause
 *   or a TanStack Query mutation's `error` field).
 * @param fallback - Returned as-is when `err` is not an `Error` instance.
 */
export function getErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}
