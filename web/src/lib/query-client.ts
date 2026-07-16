import { QueryClient, isServer } from "@tanstack/react-query"

/**
 * Shared `QueryClient` factory/accessor for the TanStack Query cache used
 * across Server Components (RSC prefetch), Client Components, and the
 * `QueryProvider` mounted in `app/layout.tsx`.
 *
 * Follows the standard TanStack Query Next.js App Router pattern: the
 * server always gets a fresh `QueryClient` per request (React Server
 * Components render on every request and must never share mutable state
 * across requests/users), while the browser reuses a single module-level
 * instance across re-renders so client-side navigation doesn't discard the
 * cache.
 *
 * `staleTime: 60_000` keeps data RSC-prefetched via `prefetchQuery` from
 * being immediately refetched the moment a client component mounts and
 * calls the matching `useQuery` hook.
 */

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 60 * 1000,
      },
    },
  })
}

let browserQueryClient: QueryClient | undefined

/**
 * Returns the `QueryClient` to use for the current render.
 *
 * On the server this always constructs a new client (see {@link isServer}),
 * so prefetched data never leaks between requests. In the browser, the same
 * singleton is returned on every call so the cache survives re-renders and
 * client-side navigations.
 */
export function getQueryClient(): QueryClient {
  if (isServer) {
    return makeQueryClient()
  }

  if (!browserQueryClient) {
    browserQueryClient = makeQueryClient()
  }
  return browserQueryClient
}
