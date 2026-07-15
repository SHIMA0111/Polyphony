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
 *   - `scrollIntoView`, used by
 *     `web/src/features/messages/components/MessageList.tsx`'s
 *     `endRef.current?.scrollIntoView(...)` effect.
 *   - `matchMedia`, used transitively by `next-themes`' `ThemeProvider` via
 *     `web/src/components/ui/color-mode.tsx` -> `web/src/components/ui/provider.tsx`.
 * - Start/reset/stop the MSW Node server for the whole suite so fetches made
 *   by components/hooks under test are intercepted rather than hitting the
 *   network.
 */

window.HTMLElement.prototype.scrollIntoView = vi.fn()

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
