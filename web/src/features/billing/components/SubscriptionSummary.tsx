"use client"

import Link from "next/link"
import { Badge, Box, Button, Card, Flex, Skeleton, Text } from "@chakra-ui/react"
import { formatUtcDate } from "@/lib/format"
import { useCreateBillingPortalSession } from "../hooks/use-create-billing-portal-session"
import { usePlans } from "../hooks/use-plans"
import { useSubscription } from "../hooks/use-subscription"
import type { SubscriptionStatus } from "../types"

/** `Badge` `colorPalette` per `SubscriptionStatus`. */
const STATUS_COLOR_PALETTE: Record<SubscriptionStatus, string> = {
  active: "green",
  trialing: "blue",
  past_due: "orange",
  canceled: "gray",
  none: "gray",
}

/**
 * The current user's subscription state (`useSubscription()`,
 * `GET /billing/subscription`), rendered by
 * `app/(main)/billing/subscription/page.tsx`.
 *
 * When there is no active subscription (`status === "none"`), renders a
 * prompt linking to `/billing/plans` instead of an empty card. Otherwise
 * renders the resolved plan display name (`usePlans()` lookup by
 * `plan_code`, falling back to the raw code if the catalog hasn't loaded or
 * no longer lists it), a status `Badge`, the renewal date, a
 * cancel-at-period-end notice when applicable, a "Manage subscription"
 * action (`useCreateBillingPortalSession()`), and a "Change plan" link.
 */
export function SubscriptionSummary() {
  const { data: subscription, isPending, isError } = useSubscription()
  const { data: plans } = usePlans()
  const portalMutation = useCreateBillingPortalSession()

  if (isPending) {
    return (
      <Flex direction="column" gap={2}>
        <Skeleton h="8" w="48" rounded="md" />
        <Skeleton h="32" rounded="lg" />
      </Flex>
    )
  }

  if (isError) {
    return (
      <Box borderWidth="1px" borderColor="border" rounded="lg" p={6} textAlign="center">
        <Text color="fg.error">Failed to load subscription.</Text>
      </Box>
    )
  }

  if (subscription.status === "none") {
    return (
      <Box borderWidth="1px" borderStyle="dashed" borderColor="border" rounded="lg" p={6}>
        <Text color="fg.muted" mb={3}>
          You don&apos;t have an active subscription
        </Text>
        <Link href="/billing/plans">
          <Button colorPalette="blue" size="sm">
            View plans
          </Button>
        </Link>
      </Box>
    )
  }

  const planName =
    plans?.find((plan) => plan.code === subscription.plan_code)?.name ??
    subscription.plan_code ??
    "—"

  return (
    <Card.Root>
      <Card.Body display="flex" flexDirection="column" gap={4}>
        <Flex align="center" justify="space-between" gap={2}>
          <Card.Title fontSize="lg">{planName}</Card.Title>
          <Badge
            variant="subtle"
            colorPalette={STATUS_COLOR_PALETTE[subscription.status]}
            // Own, directly-assertable test hook (see `BalanceBadge`'s
            // `data-low-balance` for the same rationale): `colorPalette`
            // resolves to a CSS custom property at render time, which
            // jsdom's computed style doesn't reliably expose.
            data-status={subscription.status}
          >
            {subscription.status}
          </Badge>
        </Flex>

        {subscription.current_period_end && (
          <Text color="fg.muted" fontSize="sm">
            {subscription.cancel_at_period_end ? "Ends" : "Renews"} on{" "}
            {formatUtcDate(subscription.current_period_end)}
          </Text>
        )}

        {subscription.monthly_token_allocation !== null && (
          <Text fontSize="sm" color="fg.muted">
            {subscription.monthly_token_allocation.toLocaleString("en-US")} tokens / month
          </Text>
        )}

        {subscription.cancel_at_period_end && subscription.current_period_end && (
          <Box borderWidth="1px" borderColor="orange.300" bg="orange.subtle" rounded="md" p={3}>
            <Text fontSize="sm" color="orange.fg">
              Your subscription will end on {formatUtcDate(subscription.current_period_end)} and
              will not renew.
            </Text>
          </Box>
        )}

        <Flex gap={3}>
          <Button
            variant="outline"
            loading={portalMutation.isPending}
            loadingText="Opening..."
            onClick={() => portalMutation.mutate()}
          >
            Manage subscription
          </Button>
          <Link href="/billing/plans">
            <Button variant="ghost">Change plan</Button>
          </Link>
        </Flex>
      </Card.Body>
    </Card.Root>
  )
}
