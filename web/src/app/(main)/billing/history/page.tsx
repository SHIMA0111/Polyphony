"use client"

import { Box, Heading } from "@chakra-ui/react"
import { PaymentHistoryList } from "@/features/billing/components/PaymentHistoryList"

/** `/billing/history` — a paginated list of the current user's past payments. */
export default function BillingHistoryPage() {
  return (
    <Box px={4} py={6}>
      <Box maxW="3xl" mx="auto">
        <Heading size="lg" mb={6}>
          Billing History
        </Heading>
        <PaymentHistoryList />
      </Box>
    </Box>
  )
}
