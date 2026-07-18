"use client"

import Link from "next/link"
import { Box, Button, Card, Flex, Heading, SimpleGrid, Text } from "@chakra-ui/react"
import { AlertCircle, Users } from "lucide-react"
import { useGroups } from "../hooks/use-groups"
import { GroupFormDialog } from "./GroupFormDialog"

/**
 * Content-pane role only, mirroring `RoomList`: the "Groups" heading, the
 * create-group entry point (`GroupFormDialog` in create mode), and the
 * group grid sourced from `useGroups()` — this is `/groups`'s page content.
 * The `isError` branch mirrors `RoomList`'s own error `Card.Root` (same
 * dashed border, icon-in-circle, and retry button) rather than the
 * "No groups yet" empty state, so a failed load reads as retriable rather
 * than as "you have nothing here".
 */
export function GroupList() {
  const { data, isPending, isError, error, refetch } = useGroups()
  const groups = data?.groups ?? []

  return (
    <Box h="100%" bg="bg">
      <Box as="main" maxW="7xl" mx="auto" px={4} py={8}>
        <Flex align="center" justify="space-between" mb={8}>
          <Box>
            <Heading size="3xl" fontWeight="bold" letterSpacing="tight">
              Groups
            </Heading>
            <Text color="fg.muted" mt={1}>
              Maintain address-book-style groups to invite in one action
            </Text>
          </Box>
          <GroupFormDialog mode="create" />
        </Flex>

        {isPending ? (
          <SimpleGrid columns={{ base: 1, md: 2, lg: 3 }} gap={4}>
            {[1, 2, 3].map((i) => (
              <Card.Root key={i} h="120px" opacity={0.5}>
                <Card.Body />
              </Card.Root>
            ))}
          </SimpleGrid>
        ) : isError ? (
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
                  <AlertCircle size={32} color="var(--chakra-colors-fg-error)" />
                </Flex>
                <Heading size="md" mb={2}>
                  Couldn&apos;t load your groups
                </Heading>
                <Text color="fg.muted" mb={6} maxW="sm" role="alert">
                  {error instanceof Error
                    ? error.message
                    : "Something went wrong while fetching your groups."}
                </Text>
                <Button onClick={() => refetch()} colorPalette="blue">
                  Try again
                </Button>
              </Flex>
            </Card.Body>
          </Card.Root>
        ) : groups.length > 0 ? (
          <SimpleGrid columns={{ base: 1, md: 2, lg: 3 }} gap={4}>
            {groups.map((group) => (
              <Link key={group.id} href={`/groups/${group.id}`}>
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
                      {group.name}
                    </Card.Title>
                    <Card.Description lineClamp={2}>
                      {group.description || "No description"}
                    </Card.Description>
                  </Card.Header>
                  <Card.Body>
                    <Flex align="center" gap={1} fontSize="sm" color="fg.muted">
                      <Users size={14} />
                      <Text>Members</Text>
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
                  <Users size={32} color="var(--chakra-colors-fg-muted)" />
                </Flex>
                <Heading size="md" mb={2}>
                  No groups yet
                </Heading>
                <Text color="fg.muted" mb={6} maxW="sm">
                  Create a group to invite the same set of people to a room
                  in one action
                </Text>
                <GroupFormDialog mode="create" />
              </Flex>
            </Card.Body>
          </Card.Root>
        )}
      </Box>
    </Box>
  )
}
