"use client"

import Link from "next/link"
import { Box, Button, Flex, Heading } from "@chakra-ui/react"
import { ChevronLeft } from "lucide-react"
import { UsageHistoryList } from "@/features/billing/components/UsageHistoryList"

/**
 * `/billing/usage` — the current user's token usage/transaction history,
 * reached by clicking the top-bar `BalanceBadge`
 * (`web/src/app/(main)/layout.tsx`).
 *
 * Read-only: no purchase/top-up UI here (Step 53's territory once the
 * Stripe billing UI, Phase 17, lands).
 */
export default function BillingUsagePage() {
  return (
    <Flex direction="column" h="full" flex={1} minW={0}>
      <Flex
        as="header"
        borderBottomWidth="1px"
        bg="bg/80"
        backdropFilter="blur(8px)"
        align="center"
        gap={3}
        px={4}
        h={16}
        flexShrink={0}
      >
        <Button asChild variant="ghost" size="sm" gap={1.5} px={2}>
          <Link href="/rooms">
            <ChevronLeft size={18} />
            Rooms
          </Link>
        </Button>
        <Heading size="md">Token Usage</Heading>
      </Flex>

      <Box flex={1} minH={0} overflowY="auto" px={4} py={6}>
        <Box maxW="3xl" mx="auto">
          <UsageHistoryList />
        </Box>
      </Box>
    </Flex>
  )
}
