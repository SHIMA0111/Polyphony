"use client"

import Link from "next/link"
import { Box, Button, Flex, Heading, Text } from "@chakra-ui/react"

/**
 * The post-Checkout redirect target when the user backs out of Stripe's
 * hosted Checkout page (`/billing/checkout/cancel`, per Step 49's Stripe
 * configuration). No charge was made — nothing to poll or reconcile here.
 */
export default function BillingCheckoutCancelPage() {
  return (
    <Box px={4} py={6}>
      <Box maxW="xl" mx="auto">
        <Flex direction="column" align="center" gap={4} py={12} textAlign="center">
          <Heading size="lg">Checkout canceled</Heading>
          <Text color="fg.muted">You have not been charged.</Text>
          <Button asChild colorPalette="blue">
            <Link href="/billing/plans">Back to plans</Link>
          </Button>
        </Flex>
      </Box>
    </Box>
  )
}
