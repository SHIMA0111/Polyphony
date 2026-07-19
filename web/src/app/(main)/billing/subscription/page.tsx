"use client"

import { Box, Heading } from "@chakra-ui/react"
import { SubscriptionSummary } from "@/features/billing/components/SubscriptionSummary"

/**
 * `/billing/subscription` — the current user's plan, status,
 * renewal/cancellation state, and "Manage subscription" action.
 */
export default function BillingSubscriptionPage() {
  return (
    <Box px={4} py={6}>
      <Box maxW="3xl" mx="auto">
        <Heading size="lg" mb={6}>
          Your Subscription
        </Heading>
        <SubscriptionSummary />
      </Box>
    </Box>
  )
}
