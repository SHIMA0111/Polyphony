import type { ReactElement } from "react"
import { render, type RenderOptions } from "@testing-library/react"
import { Provider } from "@/components/ui/provider"

/**
 * Custom React Testing Library `render` that wraps `ui` in the app's real
 * `Provider` (`web/src/components/ui/provider.tsx`, composing
 * `ChakraProvider value={defaultSystem}` + `ColorModeProvider`), so components
 * under test get the same Chakra style/token context they get at runtime
 * without each test hand-rolling a wrapper.
 *
 * Re-exports everything else from `@testing-library/react` so this module can
 * be used as a drop-in replacement: `import { renderWithProviders as render, screen } from "@/test/render"`.
 */
function renderWithProviders(ui: ReactElement, options?: RenderOptions) {
  return render(ui, { wrapper: Provider, ...options })
}

export * from "@testing-library/react"
export { renderWithProviders as render }
