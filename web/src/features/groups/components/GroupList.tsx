"use client"

import Link from "next/link"
import { Box, Button, Card, Flex, Heading, SimpleGrid, Text } from "@chakra-ui/react"
import { AlertTriangle, Users } from "lucide-react"
import { getErrorMessage } from "@/lib/get-error-message"
import { useGroups } from "../hooks/use-groups"
import { GroupFormDialog } from "./GroupFormDialog"

/**
 * Content-pane role only, mirroring `RoomList`: the "Groups" heading, the
 * create-group entry point (`GroupFormDialog` in create mode), and the
 * group grid sourced from `useGroups()` — this is `/groups`'s page content.
 * The error state (checked before the empty-state check, so a failed load
 * can't render "No groups yet" instead) mirrors `RoomList`'s own dashed
 * `Card.Root` + retry-button empty/error-state styling.
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
          <Card.Root borderStyle="dashed" borderColor="border.error">
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
                  <AlertTriangle size={32} color="var(--chakra-colors-fg-error)" />
                </Flex>
                <Heading size="md" mb={2}>
                  Failed to load groups
                </Heading>
                <Text color="fg.muted" mb={6} maxW="sm">
                  {getErrorMessage(error, "Something went wrong while loading your groups.")}
                </Text>
                <Button variant="outline" onClick={() => refetch()}>
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
