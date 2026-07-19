"use client"

import Link from "next/link"
import { Badge, Flex, Skeleton, Text } from "@chakra-ui/react"
import { Coins } from "lucide-react"
import { useBalance } from "../hooks/use-balance"

/** Balance at or below this threshold gets the low-balance visual treatment. */
const LOW_BALANCE_THRESHOLD = 0

/**
 * Compact top-bar widget showing the current user's token balance
 * (`useBalance()`, `GET /billing/balance`), linking to `/billing/usage`.
 *
 * Passive indicator only — no click-to-purchase action; that is Step 53's
 * territory once the Stripe billing UI (Phase 17) lands. States:
 * - loading: a `Skeleton` placeholder sized to the eventual badge;
 * - error: a muted "—" (the widget degrading gracefully rather than
 *   blocking the rest of the top bar on a billing-endpoint hiccup);
 * - loaded, balance <= {@link LOW_BALANCE_THRESHOLD}: `colorPalette="red"`
 *   to flag that the next AI send will likely 402;
 * - loaded, otherwise: the default `colorPalette="gray"` treatment.
 */
export function BalanceBadge() {
  const { data, isPending, isError } = useBalance()

  const isLow = data !== undefined && data.balance <= LOW_BALANCE_THRESHOLD
  const colorPalette = isLow ? "red" : "gray"

  return (
    <Link href="/billing/usage" aria-label="View token usage history">
      <Badge
        variant="subtle"
        colorPalette={colorPalette}
        // Own, directly-assertable test hook: `colorPalette` itself resolves
        // to a CSS custom property at render time, which jsdom's computed
        // style doesn't reliably expose, so tests key off this instead.
        data-low-balance={isLow}
        size="lg"
        rounded="full"
        px={3}
        py={1.5}
        gap={1.5}
        cursor="pointer"
        _hover={{ opacity: 0.85 }}
      >
        <Coins size={14} />
        {isPending ? (
          <Skeleton h="4" w="10" />
        ) : isError ? (
          <Text as="span" color="fg.muted">
            —
          </Text>
        ) : (
          <Flex as="span" align="baseline" gap={1}>
            <Text as="span" fontWeight="semibold">
              {data.balance.toLocaleString("en-US")}
            </Text>
            <Text as="span" fontSize="2xs" color="fg.muted">
              tokens
            </Text>
          </Flex>
        )}
      </Badge>
    </Link>
  )
}
