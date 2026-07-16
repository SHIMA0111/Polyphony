import type { ReactElement, ReactNode } from "react"
import { render, type RenderOptions } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Provider } from "@/components/ui/provider"

/**
 * Custom React Testing Library `render` that wraps `ui` in the app's real
 * `Provider` (`web/src/components/ui/provider.tsx`, composing
 * `ChakraProvider value={defaultSystem}` + `ColorModeProvider`) and a
 * `QueryClientProvider`, so components under test get the same Chakra
 * style/token context and TanStack Query cache they get at runtime without
 * each test hand-rolling a wrapper.
 *
 * Re-exports everything else from `@testing-library/react` so this module can
 * be used as a drop-in replacement: `import { renderWithProviders as render, screen } from "@/test/render"`.
 */

/**
 * A fresh `QueryClient` per test, with retries disabled so error-path tests
 * (e.g. a `server.use()` override returning a `5xx`) resolve immediately
 * instead of retrying with backoff.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

/**
 * Wrapper factory for `@testing-library/react`'s `renderHook`, which (unlike
 * `render`) takes its `wrapper` as a component reference rather than a
 * render-time option. Each call creates its own `QueryClient` so tests don't
 * share cache state.
 */
export function createQueryClientWrapper(
  queryClient: QueryClient = createTestQueryClient(),
) {
  return function QueryClientWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )
  }
}

interface RenderWithProvidersOptions extends Omit<RenderOptions, "wrapper"> {
  /** Supply a pre-configured `QueryClient` (e.g. to spy on its methods). */
  queryClient?: QueryClient
}

// `wrapper` is omitted (not just optional) from RenderWithProvidersOptions:
// `renderWithProviders` always supplies its own `Wrapper` below to guarantee
// every rendered component gets the app's real `Provider` +
// `QueryClientProvider`. If callers could pass `wrapper` through `options`,
// it would silently replace that mandatory wrapper instead of composing with
// it, and a test could end up rendering without Chakra/TanStack Query
// context without any type error warning it.

function renderWithProviders(
  ui: ReactElement,
  { queryClient = createTestQueryClient(), ...options }: RenderWithProvidersOptions = {},
) {
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <Provider>
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      </Provider>
    )
  }

  return render(ui, { wrapper: Wrapper, ...options })
}

export * from "@testing-library/react"
export { renderWithProviders as render }
