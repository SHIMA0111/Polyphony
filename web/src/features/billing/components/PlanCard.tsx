"use client"

import { Badge, Button, Card, Flex, Text } from "@chakra-ui/react"
import type { BillingPlan } from "../types"

/**
 * Formats `price_cents` (an integer minor-currency-unit amount, per Stripe
 * convention) as a localized currency string — never render the raw cents
 * value or a hand-rolled `/ 100` division without a formatter.
 *
 * The minor-unit exponent (number of digits after the decimal point) is
 * currency-dependent — most currencies use 2 (cents), but e.g. JPY uses 0
 * and KWD uses 3 — so it's read from the formatter's own
 * `resolvedOptions().maximumFractionDigits` rather than hardcoding `/ 100`.
 */
function formatPrice(plan: BillingPlan): string {
  const formatter = new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: plan.currency,
  })
  // `maximumFractionDigits` is typed as possibly `undefined` even though
  // `resolvedOptions()` always populates it for `style: "currency"`; `?? 2`
  // matches `Intl`'s own currency-formatting default.
  const exponent = formatter.resolvedOptions().maximumFractionDigits ?? 2
  return formatter.format(plan.price_cents / 10 ** exponent)
}

/**
 * A single purchasable catalog entry (subscription plan or one-time token
 * pack), rendered by `PlanList`'s `SimpleGrid`.
 *
 * The CTA reads "Subscribe" for a recurring (`interval === "month"`) plan or
 * "Buy tokens" for a one-time token pack. `PlanList` owns the single
 * `useCreateCheckoutSession()` mutation shared by every card (so starting a
 * checkout for one plan disables every other card's CTA too, preventing
 * concurrent Checkout sessions) and passes it down as `pending`/`onSelect`.
 */
export function PlanCard({
  plan,
  pending,
  onSelect,
}: {
  plan: BillingPlan
  /** Whether *any* card's checkout-session mutation is in flight. */
  pending: boolean
  /** Starts a Checkout session for this card's plan. */
  onSelect: () => void
}) {
  const ctaLabel = plan.interval === "month" ? "Subscribe" : "Buy tokens"

  return (
    <Card.Root h="full">
      <Card.Body display="flex" flexDirection="column" gap={4}>
        <Flex align="start" justify="space-between" gap={2}>
          <Card.Title fontSize="lg">{plan.name}</Card.Title>
          <Badge variant="subtle" colorPalette={plan.interval === "month" ? "blue" : "purple"}>
            {plan.interval === "month" ? "Monthly" : "One-time"}
          </Badge>
        </Flex>

        <Card.Description>{plan.description}</Card.Description>

        <Flex direction="column" gap={1}>
          <Text fontSize="2xl" fontWeight="bold">
            {formatPrice(plan)}
            {plan.interval === "month" && (
              <Text as="span" fontSize="sm" fontWeight="normal" color="fg.muted">
                {" "}
                / month
              </Text>
            )}
          </Text>
          <Text fontSize="sm" color="fg.muted">
            {plan.token_allowance.toLocaleString("en-US")} tokens
            {plan.interval === "month" ? " / month" : ""}
          </Text>
        </Flex>

        <Button
          mt="auto"
          colorPalette="blue"
          loading={pending}
          loadingText="Redirecting..."
          onClick={onSelect}
        >
          {ctaLabel}
        </Button>
      </Card.Body>
    </Card.Root>
  )
}
