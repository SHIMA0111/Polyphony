"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { Box, Flex, Text, type FlexProps } from "@chakra-ui/react"
import { MessageSquare, MessageSquarePlus } from "lucide-react"
import { useRooms } from "@/features/rooms/hooks/use-rooms"
import { CreateRoomForm } from "./CreateRoomForm"

interface RoomRailProps {
  /**
   * Responsive `display` value, driven by the `(main)` layout's
   * `hasActiveRoom` (whether a `roomId` route param is present) so the rail
   * collapses out of view on mobile once a room is open, while always
   * showing at `md` and above.
   */
  display?: FlexProps["display"]
}

/**
 * Compact, always-mounted vertical room list rendered by
 * `web/src/app/(main)/layout.tsx`.
 *
 * Reuses `useRooms()` (the Step 9 TanStack Query hook backing
 * `web/src/features/rooms/api/get-rooms.ts`) as its sole data source, so the
 * room list is fetched once and shared with `RoomList`'s content-pane grid
 * via the same `["rooms"]` cache entry — this component never issues its own
 * fetch. The active room is derived from the `roomId` route param (present
 * for `/rooms/[roomId]`, absent for `/rooms`) via `useParams()`, which
 * reflects whichever `(main)` page is currently rendered beneath the
 * persistent layout.
 */
export function RoomRail({ display }: RoomRailProps) {
  const { data: rooms = [], isPending } = useRooms()
  const params = useParams<{ roomId?: string }>()
  const activeRoomId = typeof params.roomId === "string" ? params.roomId : undefined

  return (
    <Flex
      as="nav"
      aria-label="Rooms"
      direction="column"
      w={{ base: "full", md: "280px" }}
      flexShrink={0}
      h="full"
      overflowY="auto"
      borderRightWidth="1px"
      bg="bg.subtle"
      display={display}
    >
      {/* Mini-header: label + compact create-room entry point, always
          visible regardless of the room list's loading/empty/populated
          state below. */}
      <Flex
        align="center"
        justify="space-between"
        gap={2}
        px={3}
        py={2}
        flexShrink={0}
        borderBottomWidth="1px"
      >
        <Text
          fontSize="xs"
          fontWeight="semibold"
          letterSpacing="wide"
          textTransform="uppercase"
          color="fg.muted"
        >
          Rooms
        </Text>
        <CreateRoomForm />
      </Flex>

      <Box flex={1} minH={0} overflowY="auto">
        {isPending ? (
          <Flex direction="column" gap={1} p={2} aria-busy="true">
            {[1, 2, 3, 4].map((i) => (
              <Flex key={i} align="center" gap={2} px={3} py={2} opacity={0.5}>
                <Box h={4} w={4} rounded="sm" bg="bg.muted" flexShrink={0} />
                <Box h={3} flex={1} rounded="sm" bg="bg.muted" />
              </Flex>
            ))}
          </Flex>
        ) : rooms.length > 0 ? (
          <Flex direction="column" gap={0.5} p={2}>
            {rooms.map((room) => {
              const active = room.id === activeRoomId
              return (
                <Link
                  key={room.id}
                  href={`/rooms/${room.id}`}
                  aria-current={active ? "page" : undefined}
                >
                  <Flex
                    align="center"
                    gap={2}
                    px={3}
                    py={2}
                    rounded="md"
                    fontSize="sm"
                    colorPalette="blue"
                    bg={active ? "colorPalette.subtle" : "transparent"}
                    color={active ? "colorPalette.fg" : "fg"}
                    fontWeight={active ? "medium" : "normal"}
                    borderLeftWidth="3px"
                    borderLeftColor={active ? "colorPalette.solid" : "transparent"}
                    _hover={{ bg: active ? "colorPalette.subtle" : "bg.muted" }}
                    transition="background-color 0.15s, color 0.15s"
                  >
                    <MessageSquare size={16} style={{ flexShrink: 0 }} />
                    <Text truncate>{room.name}</Text>
                  </Flex>
                </Link>
              )
            })}
          </Flex>
        ) : (
          <Flex
            direction="column"
            align="center"
            justify="center"
            h="full"
            gap={3}
            px={4}
            py={10}
            textAlign="center"
          >
            <Flex
              h={12}
              w={12}
              rounded="full"
              bg="bg.muted"
              align="center"
              justify="center"
            >
              <MessageSquarePlus size={22} color="var(--chakra-colors-fg-muted)" />
            </Flex>
            <Text fontSize="sm" color="fg.muted">
              No rooms yet
            </Text>
          </Flex>
        )}
      </Box>
    </Flex>
  )
}
