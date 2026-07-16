import { Flex, Separator, Text } from "@chakra-ui/react"

interface DaySeparatorProps {
  /** Precomputed label, e.g. `"Today"`, `"Yesterday"`, or a localized date. */
  label: string
}

/** A centered pill/rule inserted between message groups on different calendar days. */
export function DaySeparator({ label }: DaySeparatorProps) {
  return (
    <Flex align="center" gap={3} my={2}>
      <Separator flex={1} borderColor="border.muted" />
      <Text
        flexShrink={0}
        fontSize="xs"
        fontWeight="medium"
        color="fg.muted"
        px={3}
        py={1}
        rounded="full"
        bg="bg.subtle"
      >
        {label}
      </Text>
      <Separator flex={1} borderColor="border.muted" />
    </Flex>
  )
}
