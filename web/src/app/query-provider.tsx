"use client"

import { useState } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { ReactQueryDevtools } from "@tanstack/react-query-devtools"
import { getQueryClient } from "@/lib/query-client"

/**
 * App-wide TanStack Query provider, mounted once in `app/layout.tsx`.
 *
 * `useState(() => getQueryClient())` guarantees the `QueryClient` instance
 * is created exactly once per component instance (not on every render),
 * while still deferring to {@link getQueryClient}'s server/browser split so
 * server renders get a fresh client and the browser reuses its singleton
 * across re-renders.
 *
 * Devtools are only mounted outside production so the bundle/behavior of a
 * production build is unaffected.
 */
export function QueryProvider({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => getQueryClient())

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      {process.env.NODE_ENV !== "production" && (
        <ReactQueryDevtools initialIsOpen={false} />
      )}
    </QueryClientProvider>
  )
}
