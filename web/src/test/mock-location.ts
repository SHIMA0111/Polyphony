/**
 * Test helper for asserting a full-browser-navigation `window.location.href
 * = ...` assignment (e.g. `useCreateCheckoutSession`/
 * `useCreateBillingPortalSession` redirecting to a Stripe-hosted page)
 * without jsdom attempting (and failing) a real navigation.
 *
 * Unlike a naive `{ ...window.location, href: "" }` stand-in, this keeps
 * `href` live-readable via a getter/setter pair backed by the *original*
 * `window.location.href` until a component actually assigns a new one: MSW's
 * fetch interceptor resolves this app's relative `/api/proxy/*` calls
 * against `location.href` at request time (see
 * `@mswjs/interceptors`' `fetchProxy`), so stubbing it to an empty string
 * up front breaks every relative fetch made after the stub is installed —
 * not just the one navigation this helper exists to observe.
 *
 * Usage:
 * ```ts
 * let restoreLocation: () => void
 * beforeEach(() => { restoreLocation = mockLocationHref() })
 * afterEach(() => restoreLocation())
 * // ... trigger the navigation ...
 * expect(window.location.href).toBe("https://checkout.stripe.com/...")
 * ```
 */
export function mockLocationHref(): () => void {
  const originalLocation = window.location
  let currentHref = originalLocation.href

  Object.defineProperty(window, "location", {
    configurable: true,
    value: {
      ...originalLocation,
      get href() {
        return currentHref
      },
      set href(value: string) {
        currentHref = value
      },
    },
  })

  return () => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    })
  }
}
