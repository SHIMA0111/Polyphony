"use client"

import { Badge, Button, Card, Flex, Text } from "@chakra-ui/react"
import { formatCurrency } from "@/lib/format"
import type { BillingPlan } from "../types"

/**
 * A single purchasable catalog entry (subscription plan or one-time token
 * pack), rendered by `PlanList`'s `SimpleGrid`.
 *
 * The CTA reads "Subscribe" for a recurring (`interval === "month"`) plan or
 * "Buy tokens" for a one-time token pack. The checkout mutation itself is
 * owned by `PlanList` (not this component) and reached only through
 * `onSelect` — every card in the grid shares that one mutation instance, so
 * `pending` reflects whether *any* card's checkout is in flight, not just
 * this one. That keeps a second card from starting a second, concurrent
 * Checkout Session while the first is still being created.
 */
export function PlanCard({
  plan,
  pending,
  onSelect,
}: {
  plan: BillingPlan
  pending: boolean
  onSelect: (plan: BillingPlan) => void
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
            {formatCurrency(plan.price_cents, plan.currency)}
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
          onClick={() => onSelect(plan)}
        >
          {ctaLabel}
        </Button>
      </Card.Body>
    </Card.Root>
  )
}
