"use client"

import { Badge, Button, Card, Flex, Text } from "@chakra-ui/react"
import { formatCurrency } from "@/lib/format"
import { useCreateCheckoutSession } from "../hooks/use-create-checkout-session"
import type { BillingPlan } from "../types"

/**
 * A single purchasable catalog entry (subscription plan or one-time token
 * pack), rendered by `PlanList`'s `SimpleGrid`.
 *
 * The CTA reads "Subscribe" for a recurring (`interval === "month"`) plan or
 * "Buy tokens" for a one-time token pack, and is wired directly to
 * `useCreateCheckoutSession()` — clicking it starts a Stripe Checkout
 * Session for this exact plan and, on success, navigates the browser to the
 * hosted Checkout page.
 */
export function PlanCard({ plan }: { plan: BillingPlan }) {
  const checkoutMutation = useCreateCheckoutSession()
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
          loading={checkoutMutation.isPending}
          loadingText="Redirecting..."
          onClick={() => checkoutMutation.mutate(plan)}
        >
          {ctaLabel}
        </Button>
      </Card.Body>
    </Card.Root>
  )
}
