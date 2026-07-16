"use client"

import { useEffect } from "react"
import { Button, Card, Flex, Heading, Text } from "@chakra-ui/react"
import { toaster } from "@/components/ui/toaster"

/**
 * Root-segment error boundary, per the Next.js App Router `error.tsx`
 * contract: a Client Component receiving `{ error, reset }`, rendered in
 * place of the segment tree whenever a render (or a Server Component data
 * fetch) throws.
 *
 * Announces the failure via the global toaster on mount (so it's noticed
 * even if the user's attention isn't on the full-page fallback), in addition
 * to rendering the fallback itself with a "Try again" button that calls
 * `reset()` to attempt to re-render the segment.
 */
export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    toaster.create({
      type: "error",
      title: "Something went wrong",
      description: error.message || "An unexpected error occurred.",
    })
  }, [error])

  return (
    <Flex minH="100vh" align="center" justify="center" bg="bg.subtle" p={4}>
      <Card.Root w="full" maxW="420px" shadow="lg">
        <Card.Header spaceY={3} textAlign="center">
          <Heading size="2xl" fontWeight="bold">
            Something went wrong
          </Heading>
        </Card.Header>
        <Card.Body>
          <Text textAlign="center" color="fg.muted" mb={6}>
            {error.message || "An unexpected error occurred. Please try again."}
          </Text>
          <Button
            colorPalette="blue"
            size="lg"
            w="full"
            onClick={() => reset()}
          >
            Try again
          </Button>
        </Card.Body>
      </Card.Root>
    </Flex>
  )
}
