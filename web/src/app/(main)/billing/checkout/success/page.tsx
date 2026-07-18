"use client"

import { Suspense, useEffect, useState } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Box, Button, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { AlertTriangle, CheckCircle2 } from "lucide-react"
import { usePlans } from "@/features/billing/hooks/use-plans"
import { useSubscription } from "@/features/billing/hooks/use-subscription"
import { useBalance } from "@/features/billing/hooks/use-balance"
import { readAndClearCheckoutMarker, type CheckoutMarker } from "@/features/billing/lib/checkout-marker"
import { getErrorMessage } from "@/lib/get-error-message"
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
 * after this redirect) is what actually applies a purchase's effect —
 * either the subscription or the token balance, depending on what was
 * bought. This page can't tell which from the URL alone (Stripe's redirect
 * carries only `session_id`), so it reads the `CheckoutMarker` that
 * `useCreateCheckoutSession` wrote to `sessionStorage` right before
 * redirecting here and branches on its `kind`:
 *
 * - `"subscription"` — poll `useSubscription()` until its status is no
 *   longer `"none"` (`SubscriptionPolling`).
 * - `"token_purchase"` — a token pack never changes the subscription, only
 *   the balance, so instead poll `useBalance()` until it exceeds the
 *   marker's `priorBalance` snapshot (`TokenPurchasePolling`).
 * - no marker (e.g. a direct/bookmarked visit, or an older tab that started
 *   checkout before this marker existed) — render a neutral success message
 *   with links to `/billing` rather than a false "still processing"
 *   (`NeutralSuccess`).
 *
 * Both polling branches invalidate their respective query on mount (a
 * purchase can change either), cap retries at {@link MAX_POLL_ATTEMPTS}
 * spaced {@link POLL_INTERVAL_MS} apart, and render a distinct retryable
 * error state if the query itself fails rather than silently looping.
 */
function CheckoutSuccessContent() {
  const searchParams = useSearchParams()
  const sessionId = searchParams.get("session_id")

  // Read (and clear) once on mount: the marker is single-use, and re-reading
  // it on every render would defeat its own "clear on read" staleness
  // guard.
  const [marker] = useState<CheckoutMarker | null>(() => readAndClearCheckoutMarker())

  if (marker === null) {
    return <NeutralSuccess sessionId={sessionId} />
  }

  if (marker.kind === "subscription") {
    return <SubscriptionPolling sessionId={sessionId} />
  }

  return <TokenPurchasePolling sessionId={sessionId} priorBalance={marker.priorBalance} />
}

/** Shared centered-column shell every branch below renders into. */
function StatusLayout({ children }: { children: React.ReactNode }) {
  return (
    <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
      {children}
    </Flex>
  )
}

/** The "still processing after {@link MAX_POLL_ATTEMPTS} attempts" fallback, shared by both polling branches. */
function ProcessingFallback({
  billingHref,
  onCheckAgain,
}: {
  billingHref: string
  onCheckAgain: () => void
}) {
  return (
    <StatusLayout>
      <Heading size="lg">Your payment is processing</Heading>
      <Text color="fg.muted">This can take a moment — check back shortly.</Text>
      <Button onClick={onCheckAgain}>Check again</Button>
      <Button asChild variant="ghost" size="sm">
        <Link href={billingHref}>Go to billing</Link>
      </Button>
    </StatusLayout>
  )
}

/** The retryable error state, shared by both polling branches when their query fails. */
function ErrorFallback({
  message,
  sessionId,
  onRetry,
}: {
  message: string
  sessionId: string | null
  onRetry: () => void
}) {
  return (
    <StatusLayout>
      <Box color="fg.error">
        <AlertTriangle size={48} />
      </Box>
      <Heading size="lg">Something went wrong</Heading>
      <Text color="fg.muted">{message}</Text>
      <Button onClick={onRetry}>Try again</Button>
      <Button asChild variant="ghost" size="sm">
        <Link href="/billing">Go to billing</Link>
      </Button>
      {sessionId && (
        <Text fontSize="xs" color="fg.muted" mt={4}>
          Order reference: {sessionId}
        </Text>
      )}
    </StatusLayout>
  )
}

/** No marker was found — render a neutral, non-erroring success message rather than guessing what was bought. */
function NeutralSuccess({ sessionId }: { sessionId: string | null }) {
  return (
    <StatusLayout>
      <Box color="green.fg">
        <CheckCircle2 size={48} />
      </Box>
      <Heading size="lg">Payment received</Heading>
      <Text color="fg.muted">Your purchase is being processed.</Text>
      <Flex gap={3} mt={2}>
        <Button asChild colorPalette="blue">
          <Link href="/billing">Go to billing</Link>
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
    </StatusLayout>
  )
}

/** Polls `useSubscription()` until its status is no longer `"none"`, for a `plan.interval === "month"` purchase. */
function SubscriptionPolling({ sessionId }: { sessionId: string | null }) {
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: ["billing", "subscription"] })
  }, [queryClient])

  const { data: subscription, isPending, isError, error, refetch } = useSubscription()
  const { data: plans } = usePlans()

  const resolved = subscription !== undefined && subscription.status !== "none"
  const exhausted = !resolved && attempts >= MAX_POLL_ATTEMPTS

  useEffect(() => {
    if (isPending || isError || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetch()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [isPending, isError, resolved, exhausted, attempts, refetch])

  if (isError) {
    return (
      <ErrorFallback
        message={getErrorMessage(error, "We couldn't confirm your subscription.")}
        sessionId={sessionId}
        onRetry={() => refetch()}
      />
    )
  }

  if (resolved) {
    const planName =
      plans?.find((plan) => plan.code === subscription.plan_code)?.name ??
      subscription.plan_code ??
      "—"

    return (
      <StatusLayout>
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
      </StatusLayout>
    )
  }

  if (exhausted) {
    return (
      <ProcessingFallback
        billingHref="/billing/subscription"
        onCheckAgain={() => {
          setAttempts(0)
          refetch()
        }}
      />
    )
  }

  return (
    <StatusLayout>
      <Spinner size="lg" />
      <Text color="fg.muted">Confirming your payment...</Text>
    </StatusLayout>
  )
}

/**
 * Polls `useBalance()` until it exceeds `priorBalance`, for a
 * `plan.interval === "one_time"` (token pack) purchase — this purchase kind
 * never changes the subscription, so polling that (as the pre-fix version
 * of this page always did) would loop until {@link MAX_POLL_ATTEMPTS} every
 * time.
 */
function TokenPurchasePolling({
  sessionId,
  priorBalance,
}: {
  sessionId: string | null
  priorBalance: number
}) {
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })
  }, [queryClient])

  const { data: balance, isPending, isError, error, refetch } = useBalance()

  const resolved = balance !== undefined && balance.balance > priorBalance
  const exhausted = !resolved && attempts >= MAX_POLL_ATTEMPTS

  useEffect(() => {
    if (isPending || isError || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetch()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [isPending, isError, resolved, exhausted, attempts, refetch])

  if (isError) {
    return (
      <ErrorFallback
        message={getErrorMessage(error, "We couldn't confirm your token purchase.")}
        sessionId={sessionId}
        onRetry={() => refetch()}
      />
    )
  }

  if (resolved) {
    return (
      <StatusLayout>
        <Box color="green.fg">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">Tokens added</Heading>
        <Text color="fg.muted">
          Your balance is now{" "}
          <Text as="span" fontWeight="semibold">
            {balance.balance.toLocaleString("en-US")}
          </Text>{" "}
          tokens.
        </Text>
        <Flex gap={3} mt={2}>
          <Button asChild colorPalette="blue">
            <Link href="/billing">View balance</Link>
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
      </StatusLayout>
    )
  }

  if (exhausted) {
    return (
      <ProcessingFallback
        billingHref="/billing"
        onCheckAgain={() => {
          setAttempts(0)
          refetch()
        }}
      />
    )
  }

  return (
    <StatusLayout>
      <Spinner size="lg" />
      <Text color="fg.muted">Confirming your payment...</Text>
    </StatusLayout>
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
