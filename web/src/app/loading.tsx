import { Center, Spinner } from "@chakra-ui/react"

/**
 * Root-segment loading skeleton, per the Next.js App Router `loading.tsx`
 * contract: a plain Server Component shown while a route segment (or the
 * Suspense boundary Next.js wraps it in) is still resolving — e.g. during an
 * RSC data-fetch/prefetch on route transition.
 */
export default function Loading() {
  return (
    <Center minH="100vh" bg="bg">
      <Spinner size="xl" color="blue.solid" aria-label="Loading" />
    </Center>
  )
}
