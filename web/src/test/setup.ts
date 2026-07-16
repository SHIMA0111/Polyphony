import "@testing-library/jest-dom/vitest"
import { cleanup } from "@testing-library/react"
import { afterAll, afterEach, beforeAll, vi } from "vitest"
import { server } from "./msw/server"

/**
 * Global Vitest setup, wired in via `vitest.config.ts`'s `test.setupFiles`.
 *
 * Responsibilities:
 * - Load `@testing-library/jest-dom`'s matchers (e.g. `toBeInTheDocument`).
 * - Run React Testing Library's `cleanup()` after every test to unmount
 *   components and avoid cross-test leakage.
 * - Polyfill jsdom APIs real components in this repo rely on but jsdom does
 *   not implement:
 *   - `scrollTo`, used by
 *     `web/src/features/messages/hooks/use-near-bottom-scroll.ts` to move
 *     the transcript container to its bottom.
 *   - `scrollIntoView`, kept for any other component that scrolls an element
 *     into view.
 *   - `matchMedia`, used transitively by `next-themes`' `ThemeProvider` via
 *     `web/src/components/ui/color-mode.tsx` -> `web/src/components/ui/provider.tsx`.
 *   - `ResizeObserver`, used by `@zag-js/popper` (via `@floating-ui/dom`'s
 *     `autoUpdate`) to reposition any open Chakra `Popover`/`Menu`/`Select`
 *     while its trigger's size changes -- jsdom does not implement it, and an
 *     open popover otherwise throws an uncaught `ReferenceError` on the next
 *     animation frame.
 *   - `URL.createObjectURL`/`URL.revokeObjectURL`, used by
 *     `use-attachment-staging.ts` to generate/clean up a staged attachment's
 *     preview thumbnail -- jsdom does not implement either.
 * - Start/reset/stop the MSW Node server for the whole suite so fetches made
 *   by components/hooks under test are intercepted rather than hitting the
 *   network.
 */

window.HTMLElement.prototype.scrollIntoView = vi.fn()
window.HTMLElement.prototype.scrollTo = vi.fn()

class ResizeObserverStub {
  observe = vi.fn()
  unobserve = vi.fn()
  disconnect = vi.fn()
}
window.ResizeObserver = window.ResizeObserver ?? (ResizeObserverStub as unknown as typeof ResizeObserver)

let mockObjectUrlCounter = 0
window.URL.createObjectURL =
  window.URL.createObjectURL ?? vi.fn(() => `blob:mock-preview-url-${mockObjectUrlCounter++}`)
window.URL.revokeObjectURL = window.URL.revokeObjectURL ?? vi.fn()

Object.defineProperty(window, "matchMedia", {
  writable: true,
  configurable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

beforeAll(() => server.listen({ onUnhandledRequest: "error" }))
afterEach(() => {
  server.resetHandlers()
  cleanup()
})
afterAll(() => server.close())
