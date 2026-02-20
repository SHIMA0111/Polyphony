"use client"

import { IS_MOCK } from "@/lib/api"
import { Badge, Box } from "@chakra-ui/react"

export function MockBadge() {
  if (!IS_MOCK) return null

  return (
    <Box position="fixed" bottom="3" left="3" zIndex="toast">
      <Badge colorPalette="orange" variant="solid" size="sm">
        Mock Mode
      </Badge>
    </Box>
  )
}
