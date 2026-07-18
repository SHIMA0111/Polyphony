import Link from "next/link"
import { Box, Flex } from "@chakra-ui/react"

/** The billing sub-navigation's tabs, in display order. */
const BILLING_NAV_ITEMS = [
  { label: "Plans", href: "/billing/plans" },
  { label: "Subscription", href: "/billing/subscription" },
  { label: "History", href: "/billing/history" },
  { label: "Usage", href: "/billing/usage" },
] as const

/**
 * Segment layout for every route under `/billing/*`. Renders a small
 * sub-navigation row (Plans / Subscription / History / Usage) above
 * `{children}`, so Step 48's existing `/billing/usage` route gets this
 * sub-nav "for free" without any edit to its own `page.tsx`.
 *
 * A plain server component — no `"use client"` needed, since the nav itself
 * has no interactive state (no active-tab highlighting driven by
 * `usePathname()`); each link's own page renders its own heading to
 * indicate where the user is.
 */
export default function BillingLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <Flex direction="column" h="full" flex={1} minW={0}>
      <Flex
        as="nav"
        borderBottomWidth="1px"
        bg="bg/80"
        backdropFilter="blur(8px)"
        gap={1}
        px={4}
        h={12}
        align="center"
        flexShrink={0}
      >
        {BILLING_NAV_ITEMS.map((item) => (
          <Link key={item.href} href={item.href}>
            <Box
              px={3}
              py={1.5}
              rounded="md"
              fontSize="sm"
              fontWeight="medium"
              color="fg.muted"
              _hover={{ bg: "bg.muted", color: "fg" }}
            >
              {item.label}
            </Box>
          </Link>
        ))}
      </Flex>

      <Box flex={1} minH={0} overflowY="auto">
        {children}
      </Box>
    </Flex>
  )
}
