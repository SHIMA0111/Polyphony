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
 *
 * `error.message` is deliberately never rendered (it can carry internal
 * details — stack traces, upstream error text — that shouldn't reach end
 * users); both UI surfaces below show a fixed generic message instead.
 * `console.error(error)` still logs the full error (including `digest`, the
 * ID Next.js correlates with its server-side log entry) for local/CI
 * debugging. No telemetry sink is wired up yet (Phase 23).
 */
const GENERIC_ERROR_MESSAGE = "An unexpected error occurred. Please try again."

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error(error)
    toaster.create({
      type: "error",
      title: "Something went wrong",
      description: GENERIC_ERROR_MESSAGE,
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
            {GENERIC_ERROR_MESSAGE}
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
