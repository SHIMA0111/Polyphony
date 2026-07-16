"use client"

import Link from "next/link"
import { Box, Button, Flex, Heading } from "@chakra-ui/react"
import { ChevronLeft, MoreVertical } from "lucide-react"

interface ChatRoomHeaderProps {
  roomName: string | undefined
}

/**
 * The chat room page header: back-to-rooms button, truncated room name, and
 * the "more" action button. Extracted verbatim out of `ChatRoom.tsx`'s
 * header `Flex` — no new menu behavior is added here.
 *
 * The back button is hidden on desktop widths (`md` and up), where the room
 * rail provides room navigation, and only shown on mobile widths where the
 * rail is collapsed.
 */
export function ChatRoomHeader({ roomName }: ChatRoomHeaderProps) {
  return (
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
      <Button
        asChild
        variant="ghost"
        size="sm"
        p={0}
        display={{ base: "flex", md: "none" }}
        aria-label="Back to rooms"
      >
        <Link href="/rooms">
          <ChevronLeft size={20} />
        </Link>
      </Button>
      <Box flex={1} minW={0}>
        <Heading size="md" truncate>
          {roomName ?? "Chat Room"}
        </Heading>
      </Box>
      <Button variant="ghost" size="sm" p={0} aria-label="Room settings">
        <MoreVertical size={20} />
      </Button>
    </Flex>
  )
}
