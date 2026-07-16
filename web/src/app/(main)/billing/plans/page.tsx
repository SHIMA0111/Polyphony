"use client"

import { Box, Heading } from "@chakra-ui/react"
import { PlanList } from "@/features/billing/components/PlanList"

/**
 * `/billing/plans` — subscription plans and one-time token packs, with
 * "Subscribe"/"Buy tokens" CTAs that start a Stripe Checkout Session.
 */
export default function BillingPlansPage() {
  return (
    <Box px={4} py={6}>
      <Box maxW="5xl" mx="auto">
        <Heading size="lg" mb={6}>
          Plans &amp; Token Packs
        </Heading>
        <PlanList />
      </Box>
    </Box>
  )
}
