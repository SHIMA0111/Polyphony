"use client"

import { Suspense, useEffect, useState } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Box, Button, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { CheckCircle2 } from "lucide-react"
import { usePlans } from "@/features/billing/hooks/use-plans"
import { useSubscription } from "@/features/billing/hooks/use-subscription"
import { formatUtcDate } from "@/lib/format"

/** Interval between polling attempts while waiting for the webhook to land. */
const POLL_INTERVAL_MS = 2_000
/** Caps how long this page polls before showing the non-erroring fallback. */
const MAX_POLL_ATTEMPTS = 8

/**
 * The post-Checkout redirect target
 * (`/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}`, per Step
 * 49's Stripe configuration).
 *
 * Stripe's webhook (processed server-side, asynchronously, possibly seconds
 * after this redirect) is what actually updates the subscription — so this
 * page invalidates the `["billing", "subscription"]` and `["billing",
 * "balance"]` queries on mount (a purchase can change either) and then
 * polls `useSubscription()` every {@link POLL_INTERVAL_MS} until its status
 * is no longer `"none"`, up to {@link MAX_POLL_ATTEMPTS} attempts. If the
 * webhook hasn't landed by then, it shows a non-erroring "still processing"
 * fallback with a manual "Check again" button rather than reporting a
 * false failure.
 */
function CheckoutSuccessContent() {
  const searchParams = useSearchParams()
  const sessionId = searchParams.get("session_id")
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  // Runs once on mount (React Query's `QueryClient` instance is stable for
  // the lifetime of the app, so `queryClient` never actually changes): a
  // successful Checkout can change both the subscription and the token
  // balance (a token-pack purchase credits tokens immediately), so both
  // queries are invalidated here rather than relying solely on
  // `useBalance()`'s own poll interval.
  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: ["billing", "subscription"] })
    queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })
  }, [queryClient])

  const { data: subscription, isPending, refetch } = useSubscription()
  const { data: plans } = usePlans()

  const resolved = subscription !== undefined && subscription.status !== "none"
  const exhausted = !resolved && attempts >= MAX_POLL_ATTEMPTS

  useEffect(() => {
    if (isPending || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetch()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [isPending, resolved, exhausted, attempts, refetch])

  if (resolved) {
    const planName =
      plans?.find((plan) => plan.code === subscription.plan_code)?.name ??
      subscription.plan_code ??
      "—"

    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Box color="green.fg">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">You&apos;re all set</Heading>
        <Text color="fg.muted">
          You&apos;re now subscribed to <Text as="span" fontWeight="semibold">{planName}</Text>
          {subscription.current_period_end && (
            <>
              {" "}
              — renews on {formatUtcDate(subscription.current_period_end)}
            </>
          )}
          .
        </Text>
        <Flex gap={3} mt={2}>
          <Link href="/billing/subscription">
            <Button colorPalette="blue">View subscription</Button>
          </Link>
          <Link href="/rooms">
            <Button variant="outline">Back to rooms</Button>
          </Link>
        </Flex>
        {sessionId && (
          <Text fontSize="xs" color="fg.muted" mt={4}>
            Order reference: {sessionId}
          </Text>
        )}
      </Flex>
    )
  }

  if (exhausted) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Heading size="lg">Your payment is processing</Heading>
        <Text color="fg.muted">This can take a moment — check back shortly.</Text>
        <Button
          onClick={() => {
            setAttempts(0)
            refetch()
          }}
        >
          Check again
        </Button>
        <Link href="/billing/subscription">
          <Button variant="ghost" size="sm">
            Go to subscription
          </Button>
        </Link>
      </Flex>
    )
  }

  return (
    <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
      <Spinner size="lg" />
      <Text color="fg.muted">Confirming your payment...</Text>
    </Flex>
  )
}

export default function BillingCheckoutSuccessPage() {
  return (
    <Box px={4} py={6}>
      <Box maxW="xl" mx="auto">
        <Suspense
          fallback={
            <Flex justify="center" py={12}>
              <Spinner size="lg" />
            </Flex>
          }
        >
          <CheckoutSuccessContent />
        </Suspense>
      </Box>
    </Box>
  )
}
