"use client"

import { useState, useEffect, useCallback } from "react"
import Link from "next/link"
import {
  Avatar,
  Box,
  Button,
  Card,
  Flex,
  Heading,
  Menu,
  Portal,
  SimpleGrid,
  Text,
} from "@chakra-ui/react"
import { MessageSquare, Users, LogOut, Settings, Pen } from "lucide-react"
import { apiClient } from "@/lib/api"
import { useAuth } from "@/hooks/use-auth"
import type { Room } from "@/types/api"
import { CreateRoomForm } from "./CreateRoomForm"

export function RoomList() {
  const [rooms, setRooms] = useState<Room[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const { logout } = useAuth()

  const fetchRooms = useCallback(async () => {
    try {
      const data = await apiClient.listRooms()
      setRooms(data)
    } catch {
      // TODO: handle error
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchRooms()
  }, [fetchRooms])

  const handleRoomCreated = (room: Room) => {
    setRooms((prev) => [room, ...prev])
  }

  return (
    <Box minH="100vh" bg="bg">
      {/* Header */}
      <Box
        as="header"
        borderBottomWidth="1px"
        bg="bg/80"
        backdropFilter="blur(8px)"
        position="sticky"
        top={0}
        zIndex={10}
      >
        <Flex
          maxW="7xl"
          mx="auto"
          px={4}
          h={16}
          align="center"
          justify="space-between"
        >
          <Flex align="center" gap={2}>
            <Flex
              h={8}
              w={8}
              rounded="lg"
              bg="colorPalette.solid"
              align="center"
              justify="center"
              colorPalette="blue"
            >
              <Pen size={18} color="white" />
            </Flex>
            <Heading size="lg" fontWeight="bold">
              Polyphony
            </Heading>
          </Flex>

          <Menu.Root>
            <Menu.Trigger asChild>
              <Button variant="ghost" rounded="full" p={0} h={9} w={9}>
                <Avatar.Root size="sm" colorPalette="blue">
                  <Avatar.Fallback name="User" />
                </Avatar.Root>
              </Button>
            </Menu.Trigger>
            <Portal>
              <Menu.Positioner>
                <Menu.Content w="56">
                  <Menu.Item value="settings" gap={2}>
                    <Settings size={16} />
                    Settings
                  </Menu.Item>
                  <Menu.Separator />
                  <Menu.Item
                    value="logout"
                    color="fg.error"
                    gap={2}
                    onClick={logout}
                  >
                    <LogOut size={16} />
                    Log out
                  </Menu.Item>
                </Menu.Content>
              </Menu.Positioner>
            </Portal>
          </Menu.Root>
        </Flex>
      </Box>

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
          <CreateRoomForm onCreated={handleRoomCreated} />
        </Flex>

        {isLoading ? (
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
                        {new Date(room.created_at).toLocaleDateString("en-US", {
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
                <CreateRoomForm onCreated={handleRoomCreated} />
              </Flex>
            </Card.Body>
          </Card.Root>
        )}
      </Box>
    </Box>
  )
}
