"use client"

import Link from "next/link"
import { Box, Card, Flex, Heading, SimpleGrid, Text } from "@chakra-ui/react"
import { MessageSquare, Users } from "lucide-react"
import { formatUtcDate } from "@/lib/format"
import { useRooms } from "@/features/rooms/hooks/use-rooms"
import { CreateRoomForm } from "./CreateRoomForm"

/**
 * Content-pane role only: the "Your Rooms" heading, `CreateRoomForm`, and
 * the room grid. The top bar (logo, avatar menu) that used to live here has
 * moved to `web/src/app/(main)/layout.tsx` so it renders once and persists
 * across all `(main)` routes instead of once per page (Step 16).
 *
 * `h="100%"` (rather than the pre-Step-16 `minH="100vh"`) so this component
 * fills its layout-provided content pane instead of assuming it owns the
 * full viewport height; the pane's own `overflowY="auto"` still scrolls a
 * room list taller than the available height.
 */
export function RoomList() {
  const { data: rooms = [], isPending } = useRooms()

  return (
    <Box h="100%" bg="bg">
      {/* Main Content */}
      <Box as="main" maxW="7xl" mx="auto" px={4} py={8}>
        <Flex align="center" justify="space-between" mb={8}>
          <Box>
            <Heading size="3xl" fontWeight="bold" letterSpacing="tight">
              Your Rooms
            </Heading>
            <Text color="fg.muted" mt={1}>
              Collaborate with your team using AI
            </Text>
          </Box>
          <CreateRoomForm />
        </Flex>

        {isPending ? (
          <SimpleGrid columns={{ base: 1, md: 2 }} gap={4}>
            {[1, 2, 3, 4].map((i) => (
              <Card.Root key={i} h="160px" opacity={0.5}>
                <Card.Body />
              </Card.Root>
            ))}
          </SimpleGrid>
        ) : rooms.length > 0 ? (
          <SimpleGrid columns={{ base: 1, md: 2 }} gap={4}>
            {rooms.map((room) => (
              <Link key={room.id} href={`/rooms/${room.id}`}>
                <Card.Root
                  h="full"
                  cursor="pointer"
                  transition="all 0.2s"
                  _hover={{ shadow: "md", borderColor: "blue.200" }}
                >
                  <Card.Header>
                    <Card.Title
                      transition="colors 0.2s"
                      _groupHover={{ color: "blue.500" }}
                    >
                      {room.name}
                    </Card.Title>
                    <Card.Description lineClamp={2}>
                      {room.description}
                    </Card.Description>
                  </Card.Header>
                  <Card.Body>
                    <Flex
                      align="center"
                      justify="space-between"
                      fontSize="sm"
                      color="fg.muted"
                    >
                      <Flex align="center" gap={1}>
                        <Users size={14} />
                        <Text>Members</Text>
                      </Flex>
                      <Text>
                        {formatUtcDate(room.created_at, {
                          month: "short",
                          day: "numeric",
                        })}
                      </Text>
                    </Flex>
                  </Card.Body>
                </Card.Root>
              </Link>
            ))}
          </SimpleGrid>
        ) : (
          <Card.Root borderStyle="dashed">
            <Card.Body>
              <Flex
                direction="column"
                align="center"
                justify="center"
                py={16}
                textAlign="center"
              >
                <Flex
                  h={16}
                  w={16}
                  rounded="full"
                  bg="bg.subtle"
                  align="center"
                  justify="center"
                  mb={4}
                >
                  <MessageSquare size={32} color="var(--chakra-colors-fg-muted)" />
                </Flex>
                <Heading size="md" mb={2}>
                  No rooms yet
                </Heading>
                <Text color="fg.muted" mb={6} maxW="sm">
                  Create your first room to start collaborating with your team
                  using AI
                </Text>
                <CreateRoomForm />
              </Flex>
            </Card.Body>
          </Card.Root>
        )}
      </Box>
    </Box>
  )
}
