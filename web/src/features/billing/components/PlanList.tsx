"use client"

import { Box, Card, SimpleGrid, Text } from "@chakra-ui/react"
import { usePlans } from "../hooks/use-plans"
import { PlanCard } from "./PlanCard"

/** Number of skeleton cards shown while the catalog is loading. */
const SKELETON_CARD_COUNT = 3

/**
 * The full purchasable catalog (subscription plans and one-time token
 * packs), sourced from `usePlans()` (`GET /billing/plans`), rendered by
 * `app/(main)/billing/plans/page.tsx`.
 */
export function PlanList() {
  const { data: plans, isPending, isError } = usePlans()

  if (isPending) {
    return (
      <SimpleGrid columns={{ base: 1, md: 2, lg: 3 }} gap={4}>
        {Array.from({ length: SKELETON_CARD_COUNT }).map((_, i) => (
          <Card.Root key={i} h="220px" opacity={0.5}>
            <Card.Body />
          </Card.Root>
        ))}
      </SimpleGrid>
    )
  }

  if (isError) {
    return (
      <Box borderWidth="1px" borderColor="border" rounded="lg" p={6} textAlign="center">
        <Text color="fg.error">Failed to load plans.</Text>
      </Box>
    )
  }

  if (plans.length === 0) {
    return (
      <Box
        borderWidth="1px"
        borderStyle="dashed"
        borderColor="border"
        rounded="lg"
        p={6}
        textAlign="center"
      >
        <Text color="fg.muted">No plans available</Text>
      </Box>
    )
  }

  return (
    <SimpleGrid columns={{ base: 1, md: 2, lg: 3 }} gap={4}>
      {plans.map((plan) => (
        <PlanCard key={plan.code} plan={plan} />
      ))}
    </SimpleGrid>
  )
}
