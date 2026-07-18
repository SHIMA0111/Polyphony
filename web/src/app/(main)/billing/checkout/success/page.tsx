"use client"

import { Suspense, useEffect, useState } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Box, Button, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { CheckCircle2 } from "lucide-react"
import { getBalanceQueryOptions } from "@/features/billing/api/get-balance"
import { usePlans } from "@/features/billing/hooks/use-plans"
import { useSubscription } from "@/features/billing/hooks/use-subscription"
import { readAndClearCheckoutMarker, type CheckoutMarker } from "@/features/billing/utils/checkout-marker"

/** Interval between polling attempts while waiting for the webhook to land. */
const POLL_INTERVAL_MS = 2_000
/** Caps how long this page polls before showing the non-erroring fallback. */
const MAX_POLL_ATTEMPTS = 8

/**
 * `toLocaleDateString` formatting pinned to `timeZone: "UTC"`, matching
 * `RoomList.tsx`'s date-formatting convention.
 */
function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    timeZone: "UTC",
  })
}

/**
 * The post-Checkout redirect target
 * (`/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}`, per Step
 * 49's Stripe configuration).
 *
 * Stripe's webhook (processed server-side, asynchronously, possibly seconds
 * after this redirect) is what actually applies a purchase — so which query
 * this page polls depends on what was actually purchased, read once from the
 * `checkout-marker.ts` marker `useCreateCheckoutSession` writes immediately
 * before the redirect to Stripe:
 *
 * - `{ kind: "subscription" }` — polls `useSubscription()` every
 *   {@link POLL_INTERVAL_MS} until its status is no longer `"none"`.
 * - `{ kind: "token_purchase", priorBalance }` — a token pack never changes
 *   the subscription, so instead polls the balance query until it exceeds
 *   `priorBalance`.
 * - marker missing (a direct/bookmarked visit, or `sessionStorage`
 *   unavailable) — renders a neutral success message with no polling at
 *   all, rather than a false "still processing" state that can never
 *   resolve.
 *
 * If the relevant query errors outright, this renders a distinct retryable
 * error state (a manual "Try again" button) instead of silently continuing
 * to re-poll an endpoint that's already failing. If polling instead simply
 * hasn't resolved after {@link MAX_POLL_ATTEMPTS}, it shows a non-erroring
 * "still processing" fallback with its own manual "Check again" button.
 */
function CheckoutSuccessContent() {
  const searchParams = useSearchParams()
  const sessionId = searchParams.get("session_id")
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  // Read (and clear) exactly once via lazy `useState` initialization — the
  // marker is written once, right before the redirect to Stripe, and must
  // not be re-read (it's already gone from `sessionStorage`) on subsequent
  // renders of this same page visit.
  const [marker] = useState<CheckoutMarker | null>(() => readAndClearCheckoutMarker())
  const pollingKind = marker?.kind ?? null

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

  const {
    data: subscription,
    isPending: subscriptionIsPending,
    isError: subscriptionIsError,
    error: subscriptionError,
    refetch: refetchSubscription,
  } = useSubscription()
  const {
    data: balance,
    isPending: balanceIsPending,
    isError: balanceIsError,
    error: balanceError,
    refetch: refetchBalance,
  } = useQuery(getBalanceQueryOptions())
  const { data: plans } = usePlans()

  const isPending =
    pollingKind === "subscription"
      ? subscriptionIsPending
      : pollingKind === "token_purchase"
        ? balanceIsPending
        : false
  const isError =
    pollingKind === "subscription"
      ? subscriptionIsError
      : pollingKind === "token_purchase"
        ? balanceIsError
        : false
  const error =
    pollingKind === "subscription" ? subscriptionError : pollingKind === "token_purchase" ? balanceError : null
  const refetch = pollingKind === "subscription" ? refetchSubscription : refetchBalance

  // Narrows on `marker` itself (rather than the derived `pollingKind`) so
  // TypeScript can see `marker.priorBalance` is safe to read in the
  // `"token_purchase"` branch.
  const resolved =
    marker === null
      ? false
      : marker.kind === "subscription"
        ? subscription !== undefined && subscription.status !== "none"
        : balance !== undefined && balance.balance > marker.priorBalance
  const exhausted = pollingKind !== null && !resolved && attempts >= MAX_POLL_ATTEMPTS

  useEffect(() => {
    if (!pollingKind || isPending || isError || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetch()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [pollingKind, isPending, isError, resolved, exhausted, attempts, refetch])

  // No marker: either a direct/bookmarked visit or `sessionStorage` was
  // unavailable when `useCreateCheckoutSession` tried to write it. Either
  // way there's nothing to poll for, so show a neutral success message
  // rather than a "still processing" state that would never resolve.
  if (!pollingKind) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Box color="green.fg">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">Payment received</Heading>
        <Text color="fg.muted">
          Thanks — your payment was successful. It may take a moment to fully process.
        </Text>
        <Flex gap={3} mt={2}>
          <Button asChild colorPalette="blue">
            <Link href="/billing/plans">Go to billing</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/rooms">Back to rooms</Link>
          </Button>
        </Flex>
        {sessionId && (
          <Text fontSize="xs" color="fg.muted" mt={4}>
            Order reference: {sessionId}
          </Text>
        )}
      </Flex>
    )
  }

  if (isError) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Heading size="lg">Couldn&apos;t confirm your payment</Heading>
        <Text color="fg.muted">
          {error instanceof Error
            ? error.message
            : "Something went wrong while checking your payment status."}
        </Text>
        <Button
          onClick={() => {
            setAttempts(0)
            refetch()
          }}
        >
          Try again
        </Button>
        <Button asChild variant="ghost" size="sm">
          <Link href="/billing/plans">Go to billing</Link>
        </Button>
      </Flex>
    )
  }

  if (resolved && pollingKind === "subscription" && subscription) {
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
              — renews on {formatDate(subscription.current_period_end)}
            </>
          )}
          .
        </Text>
        <Flex gap={3} mt={2}>
          <Button asChild colorPalette="blue">
            <Link href="/billing/subscription">View subscription</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/rooms">Back to rooms</Link>
          </Button>
        </Flex>
        {sessionId && (
          <Text fontSize="xs" color="fg.muted" mt={4}>
            Order reference: {sessionId}
          </Text>
        )}
      </Flex>
    )
  }

  if (resolved && pollingKind === "token_purchase" && balance) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Box color="green.fg">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">Tokens added</Heading>
        <Text color="fg.muted">
          Your balance is now{" "}
          <Text as="span" fontWeight="semibold">
            {balance.balance.toLocaleString("en-US")} tokens
          </Text>
          .
        </Text>
        <Flex gap={3} mt={2}>
          <Button asChild colorPalette="blue">
            <Link href="/billing/plans">Go to billing</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/rooms">Back to rooms</Link>
          </Button>
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
        <Button asChild variant="ghost" size="sm">
          <Link href={pollingKind === "subscription" ? "/billing/subscription" : "/billing/plans"}>
            {pollingKind === "subscription" ? "Go to subscription" : "Go to billing"}
          </Link>
        </Button>
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
