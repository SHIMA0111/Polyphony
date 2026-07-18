"use client"

import { Suspense, useEffect, useState } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Box, Button, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { AlertTriangle, CheckCircle2 } from "lucide-react"
import { usePlans } from "@/features/billing/hooks/use-plans"
import { useSubscription } from "@/features/billing/hooks/use-subscription"
import { usePaymentHistory } from "@/features/billing/hooks/use-payment-history"
import { getErrorMessage } from "@/lib/get-error-message"
import { formatUtcDate } from "@/lib/format"
import type { Payment, Subscription } from "@/features/billing/types"

/** Interval between polling attempts while waiting for the webhook to land. */
const POLL_INTERVAL_MS = 2_000
/** Caps how long this page polls before showing the non-erroring fallback. */
const MAX_POLL_ATTEMPTS = 8

/**
 * The post-Checkout redirect target
 * (`/billing/checkout/success?session_id={CHECKOUT_SESSION_ID}`, per Step
 * 49's Stripe configuration — the placeholder is substituted with the real
 * Checkout Session ID by Stripe itself).
 *
 * Stripe's webhook (processed server-side, asynchronously, possibly seconds
 * after this redirect) is what actually applies a purchase's effect —
 * either the subscription or the token balance, depending on what was
 * bought. This page can't tell which from the URL alone, so — instead of
 * relying on a client-written marker from before the redirect (lost across
 * tabs, cleared sessions, or simply never written for a bookmarked/direct
 * visit) — it dual-polls two independent, server-verified signals until one
 * resolves:
 *
 * - the subscription (`useSubscription`) whose `stripe_checkout_session_id`
 *   equals this page's `session_id` — a subscription-mode Checkout Session
 *   upserts the local `Subscription` row with that field set to the
 *   session's own ID (see `BillingUsecase.upsertSubscriptionFromCheckout`'s
 *   doc comment), so this is a precise match against *this* purchase, not
 *   "any subscription happens to be active" (which would also — wrongly —
 *   resolve immediately for a returning subscriber whose existing
 *   subscription has nothing to do with the Checkout Session they just
 *   completed);
 * - a `payment_history` entry (`usePaymentHistory`) whose
 *   `stripe_reference_id` equals this page's `session_id` — the exact
 *   transaction identity for a token-purchase Checkout Session, which
 *   `handleCheckoutSessionCompleted` records with `StripeReferenceID =
 *   session.SessionID`. This is a precise match, not an inferred balance
 *   delta: a token purchase's exact `tokens_credited` comes straight from
 *   that row.
 *
 * No `session_id` — a direct/bookmarked visit, or a present-but-empty
 * `?session_id=` (trimmed and normalized to `null` below, since an empty
 * string would otherwise never match either signal above and poll forever)
 * — renders a neutral success message with links to `/billing` instead
 * (`NeutralSuccess`). Either query erroring renders its own distinct
 * retryable error state (`ErrorFallback`) rather than a single generic one,
 * and exhausting {@link MAX_POLL_ATTEMPTS} without a match renders
 * `ProcessingFallback`.
 */
function CheckoutSuccessContent() {
  const searchParams = useSearchParams()
  const sessionId = searchParams.get("session_id")?.trim() || null

  if (sessionId === null) {
    return <NeutralSuccess sessionId={null} />
  }

  return <DualPolling sessionId={sessionId} />
}

/** Shared centered-column shell every branch below renders into. */
function StatusLayout({ children }: { children: React.ReactNode }) {
  return (
    <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
      {children}
    </Flex>
  )
}

/** The "still processing after {@link MAX_POLL_ATTEMPTS} attempts" fallback. */
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

/** A retryable error state, rendered distinctly for whichever poll (subscription or payment history) failed. */
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

/** No `session_id` was present in the URL — render a neutral, non-erroring success message rather than guessing what was bought. */
function NeutralSuccess({ sessionId }: { sessionId: string | null }) {
  return (
    <StatusLayout>
      <Box color="green.fg">
        <CheckCircle2 size={48} />
      </Box>
      <Heading size="lg">Checkout finished</Heading>
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

/** The subscription resolved (see `DualPolling`) — mirrors the plan/renewal messaging a subscription purchase should show. */
function SubscriptionSuccess({
  sessionId,
  subscription,
}: {
  sessionId: string
  subscription: Subscription
}) {
  const { data: plans } = usePlans()
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
      <Text fontSize="xs" color="fg.muted" mt={4}>
        Order reference: {sessionId}
      </Text>
    </StatusLayout>
  )
}

/** A matching payment_history entry was found (see `DualPolling`) — its own `tokens_credited` is shown directly, not an inferred balance delta. */
function TokenPurchaseSuccess({ sessionId, payment }: { sessionId: string; payment: Payment }) {
  return (
    <StatusLayout>
      <Box color="green.fg">
        <CheckCircle2 size={48} />
      </Box>
      <Heading size="lg">Tokens added</Heading>
      <Text color="fg.muted">
        <Text as="span" fontWeight="semibold">
          {payment.tokens_credited.toLocaleString("en-US")}
        </Text>{" "}
        tokens were added to your balance.
      </Text>
      <Flex gap={3} mt={2}>
        <Button asChild colorPalette="blue">
          <Link href="/billing">View balance</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/rooms">Back to rooms</Link>
        </Button>
      </Flex>
      <Text fontSize="xs" color="fg.muted" mt={4}>
        Order reference: {sessionId}
      </Text>
    </StatusLayout>
  )
}

/**
 * Dual-polls `useSubscription()` and `usePaymentHistory()` until either
 * resolves this Checkout Session, or gives up after {@link
 * MAX_POLL_ATTEMPTS} — see `CheckoutSuccessContent`'s doc comment for why
 * both are needed and how each identifies "this specific session"
 * (subscription: a `stripe_checkout_session_id` match, not merely any
 * non-`"none"` status — a pre-existing subscription unrelated to this
 * Checkout Session must never resolve it; payment history: a
 * `stripe_reference_id` match). Resolution is checked before either query's
 * error state, so a transient failure on one side never masks a genuine
 * success already visible on the other.
 */
function DualPolling({ sessionId }: { sessionId: string }) {
  const queryClient = useQueryClient()
  const [attempts, setAttempts] = useState(0)

  useEffect(() => {
    queryClient.invalidateQueries({ queryKey: ["billing", "subscription"] })
    queryClient.invalidateQueries({ queryKey: ["billing", "payment-history"] })
  }, [queryClient])

  const subscriptionQuery = useSubscription()
  const paymentHistoryQuery = usePaymentHistory()

  const matchedPayment = paymentHistoryQuery.data?.pages[0]?.payments.find(
    (payment) => payment.stripe_reference_id === sessionId,
  )
  const subscriptionResolved =
    subscriptionQuery.data !== undefined &&
    subscriptionQuery.data.stripe_checkout_session_id === sessionId
  const resolved = subscriptionResolved || matchedPayment !== undefined
  const exhausted = !resolved && attempts >= MAX_POLL_ATTEMPTS
  const stillLoading = subscriptionQuery.isPending || paymentHistoryQuery.isPending
  const eitherErrored = subscriptionQuery.isError || paymentHistoryQuery.isError

  const { refetch: refetchSubscription } = subscriptionQuery
  const { refetch: refetchPaymentHistory } = paymentHistoryQuery

  useEffect(() => {
    if (stillLoading || eitherErrored || resolved || exhausted) return
    const timer = setTimeout(() => {
      setAttempts((a) => a + 1)
      refetchSubscription()
      refetchPaymentHistory()
    }, POLL_INTERVAL_MS)
    return () => clearTimeout(timer)
  }, [
    stillLoading,
    eitherErrored,
    resolved,
    exhausted,
    attempts,
    refetchSubscription,
    refetchPaymentHistory,
  ])

  if (resolved) {
    if (subscriptionResolved) {
      return <SubscriptionSuccess sessionId={sessionId} subscription={subscriptionQuery.data!} />
    }
    return <TokenPurchaseSuccess sessionId={sessionId} payment={matchedPayment!} />
  }

  if (subscriptionQuery.isError) {
    return (
      <ErrorFallback
        message={getErrorMessage(subscriptionQuery.error, "We couldn't confirm your subscription.")}
        sessionId={sessionId}
        onRetry={() => refetchSubscription()}
      />
    )
  }

  if (paymentHistoryQuery.isError) {
    return (
      <ErrorFallback
        message={getErrorMessage(paymentHistoryQuery.error, "We couldn't confirm your payment.")}
        sessionId={sessionId}
        onRetry={() => refetchPaymentHistory()}
      />
    )
  }

  if (exhausted) {
    return (
      <ProcessingFallback
        billingHref="/billing"
        onCheckAgain={() => {
          setAttempts(0)
          refetchSubscription()
          refetchPaymentHistory()
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
