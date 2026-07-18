"use client"

import { Suspense, useEffect, useState } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Box, Button, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { CheckCircle2 } from "lucide-react"
import { usePlans } from "@/features/billing/hooks/use-plans"
import { usePaymentHistory } from "@/features/billing/hooks/use-payment-history"
import { useSubscription } from "@/features/billing/hooks/use-subscription"

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
 * (`/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}`, Stripe's
 * own template placeholder, substituted with the real Checkout Session ID
 * at redirect time — see `usecase/billing.withCheckoutSessionIDParam` on the
 * server). This survives a page refresh natively, since it lives in the
 * URL rather than in a `sessionStorage` marker written before the redirect
 * (the previous mechanism, `checkout-marker.ts`, broke exactly on a
 * mid-poll refresh — it has been removed).
 *
 * Stripe's webhook (processed server-side, asynchronously, possibly seconds
 * after this redirect) is what actually applies a purchase, so this page
 * polls until server state alone can positively confirm completion. Both
 * checks below run unconditionally — this page has no marker telling it in
 * advance which kind of session `session_id` refers to, so whichever
 * resolves first wins:
 *
 * - a *recurring* purchase is confirmed once {@link useSubscription} reports
 *   a subscription whose `stripe_checkout_session_id` equals this page's
 *   `session_id` query parameter — matching Stripe's own Checkout Session
 *   identity, the same way a token purchase is matched below, rather than
 *   merely "some subscription is active" (that weaker check would also
 *   fire for a user who already had an unrelated active subscription before
 *   this particular checkout even started, e.g. one who abandons a plan
 *   upgrade's Checkout and lands here via a stale link — this page must not
 *   claim THIS session succeeded just because ANY subscription exists);
 * - a *token* purchase is confirmed once {@link usePaymentHistory} contains
 *   a row whose `stripe_reference_id` equals this page's `session_id` query
 *   parameter — matching Stripe's own Checkout Session identity, not a
 *   balance delta (a balance delta is ambiguous: unrelated activity, e.g. AI
 *   usage in another tab, could shift it too).
 *
 * A matching payment record is preferred for the *displayed* confirmation
 * (it names the exact purchase), falling back to the subscription check
 * only when no payment record matches yet — a subscription's first
 * payment_history row does not land until the later invoice.paid webhook,
 * well after checkout.session.completed.
 *
 * No `session_id` query parameter (a direct/bookmarked visit), OR one that
 * is present but empty/whitespace-only (`?session_id=`, e.g. a
 * misconfigured success URL that carries the literal query key with no
 * value substituted), renders the same neutral "purchase received" message
 * with no polling at all — never claiming a confirmed payment (nothing to
 * confirm against) or a failure (Stripe only redirects here on success), and
 * never getting stuck polling forever against an empty string no payment or
 * subscription record could ever match.
 *
 * If a polled query errors outright, this renders a distinct retryable
 * error state (a manual "Try again" button) instead of silently continuing
 * to re-poll an endpoint that's already failing. If polling instead simply
 * hasn't resolved after {@link MAX_POLL_ATTEMPTS}, it shows a non-erroring
 * "still processing" fallback with its own manual "Check again" button.
 */
function CheckoutSuccessContent() {
  const searchParams = useSearchParams()
  // Trimmed and normalized to `null` when empty/whitespace-only, so
  // `?session_id=` (present but valueless) follows the same "nothing to
  // confirm" branch as no `session_id` at all, rather than polling forever
  // against a string no payment or subscription record could ever match.
  const sessionId = searchParams.get("session_id")?.trim() || null
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  // Runs once on mount (React Query's `QueryClient` instance is stable for
  // the lifetime of the app, so `queryClient` never actually changes): a
  // successful Checkout can change subscription state, the token balance,
  // and payment history, so all three are invalidated here rather than
  // relying solely on each query's own poll/refetch interval.
  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: ["billing", "subscription"] })
    queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })
    queryClient.invalidateQueries({ queryKey: ["billing", "payment-history"] })
  }, [queryClient])

  const {
    data: subscription,
    isPending: subscriptionIsPending,
    isError: subscriptionIsError,
    error: subscriptionError,
    refetch: refetchSubscription,
  } = useSubscription()
  const {
    data: paymentHistory,
    isPending: paymentHistoryIsPending,
    isError: paymentHistoryIsError,
    error: paymentHistoryError,
    refetch: refetchPaymentHistory,
  } = usePaymentHistory()
  const { data: plans } = usePlans()

  // Matches Stripe's own Checkout Session identity (see the doc comment
  // above for why "any active subscription" is not sufficient): a
  // subscription this session did not itself create/touch must not be
  // mistaken for confirmation of this particular checkout.
  const subscriptionMatchesSession =
    sessionId !== null &&
    subscription !== undefined &&
    subscription.status !== "none" &&
    subscription.stripe_checkout_session_id === sessionId
  const matchingPayment =
    sessionId !== null
      ? paymentHistory?.pages
          .flatMap((page) => page.payments)
          .find((payment) => payment.stripe_reference_id === sessionId)
      : undefined
  const resolved = sessionId !== null && (matchingPayment !== undefined || subscriptionMatchesSession)

  const isPending = sessionId !== null && !resolved && (subscriptionIsPending || paymentHistoryIsPending)
  const isError = sessionId !== null && !resolved && (subscriptionIsError || paymentHistoryIsError)
  const error = subscriptionIsError ? subscriptionError : paymentHistoryError
  const exhausted = sessionId !== null && !resolved && !isError && attempts >= MAX_POLL_ATTEMPTS

  const retry = () => {
    setAttempts(0)
    refetchSubscription()
    refetchPaymentHistory()
  }

  useEffect(() => {
    if (!sessionId || isPending || isError || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetchSubscription()
      refetchPaymentHistory()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [sessionId, isPending, isError, resolved, exhausted, refetchSubscription, refetchPaymentHistory])

  // No session_id: either a direct/bookmarked visit or a success URL
  // configured without Stripe's placeholder. Either way there's nothing to
  // confirm against, so this branch must not CLAIM a successful payment —
  // it can only point the viewer at where their real billing state lives.
  if (sessionId === null) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Box color="fg.muted">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">Checkout finished</Heading>
        <Text color="fg.muted">
          If you just completed a checkout, it may take a moment to process. Your
          subscription and payment history show the confirmed state.
        </Text>
        <Flex gap={3} mt={2}>
          <Button asChild colorPalette="blue">
            <Link href="/billing/plans">Go to billing</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/rooms">Back to rooms</Link>
          </Button>
        </Flex>
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
        <Button onClick={retry}>Try again</Button>
        <Button asChild variant="ghost" size="sm">
          <Link href="/billing/plans">Go to billing</Link>
        </Button>
      </Flex>
    )
  }

  if (matchingPayment) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Box color="green.fg">
          <CheckCircle2 size={48} />
        </Box>
        <Heading size="lg">Tokens added</Heading>
        <Text color="fg.muted">
          Your purchase credited{" "}
          <Text as="span" fontWeight="semibold">
            {matchingPayment.tokens_credited.toLocaleString("en-US")} tokens
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
        <Text fontSize="xs" color="fg.muted" mt={4}>
          Order reference: {sessionId}
        </Text>
      </Flex>
    )
  }

  if (subscriptionMatchesSession && subscription) {
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
        <Text fontSize="xs" color="fg.muted" mt={4}>
          Order reference: {sessionId}
        </Text>
      </Flex>
    )
  }

  if (exhausted) {
    return (
      <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
        <Heading size="lg">Your payment is processing</Heading>
        <Text color="fg.muted">This can take a moment — check back shortly.</Text>
        <Button onClick={retry}>Check again</Button>
        <Button asChild variant="ghost" size="sm">
          <Link href="/billing/plans">Go to billing</Link>
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
