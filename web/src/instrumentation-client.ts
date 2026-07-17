/**
 * Next.js client instrumentation hook (`src/instrumentation-client.ts` is
 * auto-loaded before the app's own client bundle hydrates — see
 * https://nextjs.org/docs/app/api-reference/file-conventions/instrumentation-client).
 *
 * This file exists solely to make the wave-4 review's open React #418
 * ("Hydration failed because the server rendered HTML didn't match the
 * client") finding on `/rooms/[roomId]` and `/rooms` diagnosable in a real
 * (minified, production) deployment, where React's own console output is
 * reduced to a bare error code and a link to the decoder — not enough to
 * tell which component/node actually mismatched.
 *
 * React logs the *full* mismatch detail (the offending DOM node, expected
 * vs. actual text/attribute) via `console.error` immediately before
 * `onRecoverableError` fires, in both dev and prod builds — `onRecoverableError`
 * itself cannot be swapped from this file (Next.js's app-router client entry
 * wires a fixed one), so intercepting `console.error` here is the smallest,
 * version-stable way to (a) tag the exact request path and timestamp onto
 * the existing message so it's greppable in browser/error-reporting logs
 * and (b) still let the original message through unchanged for any other
 * tooling that also inspects `console.error` output.
 *
 * Remove this once the root cause above is found and fixed; it is a
 * diagnostic aid, not a fix.
 */

const HYDRATION_ERROR_CODES = ["418", "419", "421", "422", "423", "425"]

function isLikelyHydrationMismatch(args: unknown[]): boolean {
  const message = args
    .map((arg) => (typeof arg === "string" ? arg : ""))
    .join(" ")
  if (message.includes("Hydration failed") || message.includes("hydration-mismatch")) {
    return true
  }
  return HYDRATION_ERROR_CODES.some((code) =>
    message.includes(`react.dev/errors/${code}`),
  )
}

if (typeof window !== "undefined" && typeof console !== "undefined") {
  const originalConsoleError = console.error.bind(console)

  console.error = (...args: unknown[]) => {
    if (isLikelyHydrationMismatch(args)) {
      originalConsoleError(
        "[hydration-diagnostic] mismatch detected — path=%s at=%s",
        window.location.pathname,
        new Date().toISOString(),
      )
    }
    originalConsoleError(...args)
  }
}
