"use client"

import { useParams } from "next/navigation"
import { Avatar, Box, Button, Flex, Heading, Menu, Portal } from "@chakra-ui/react"
import { LogOut, Pen, Settings } from "lucide-react"
import { useLogout } from "@/features/auth/hooks/use-logout"
import { RoomRail } from "@/features/rooms/components/RoomRail"

/**
 * Persistent shell for every route under the `(main)` route group
 * (`/rooms` and `/rooms/[roomId]`).
 *
 * Renders a top bar (logo mark, "Polyphony" heading, and the avatar
 * `Menu.Root` with Settings/Logout — moved here from `RoomList` so it mounts
 * once instead of once per page) above a two-region body: the
 * always-mounted `RoomRail` room-list rail and a content pane wrapping
 * `children` (`RoomList` at `/rooms`, `ChatRoom` at `/rooms/[roomId]`).
 * Navigating between `(main)` routes only swaps `children` via Next.js
 * client-side transitions — this layout, the top bar, and `RoomRail`'s
 * `useRooms()` query never remount or refetch.
 *
 * Below the `md` breakpoint the rail and the content pane collapse into a
 * single visible region driven purely by whether a `roomId` route param is
 * present (read via `useParams()`, which reflects whichever `(main)` page is
 * rendered beneath this layout) — the same component tree renders both
 * breakpoints, there is no separate mobile-only branch.
 */
export default function MainLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const params = useParams<{ roomId?: string }>()
  const hasActiveRoom = typeof params.roomId === "string"
  const logoutMutation = useLogout()

  return (
    <Flex direction="column" h="100vh" bg="bg">
      {/* Top bar: persists across all (main) routes, never remounts on
          room-to-room or list-to-room navigation. */}
      <Box
        as="header"
        borderBottomWidth="1px"
        bg="bg/80"
        backdropFilter="blur(8px)"
        flexShrink={0}
      >
        <Flex px={4} h={16} align="center" justify="space-between">
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
                    onClick={() => logoutMutation.mutate()}
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

      {/* Rail + content pane row: collapses to a single visible region
          below `md`, driven by `hasActiveRoom`. */}
      <Flex flex={1} minH={0}>
        <RoomRail
          display={{ base: hasActiveRoom ? "none" : "flex", md: "flex" }}
        />
        <Box
          flex={1}
          minW={0}
          overflowY="auto"
          display={{ base: hasActiveRoom ? "flex" : "none", md: "flex" }}
        >
          {children}
        </Box>
      </Flex>
    </Flex>
  )
}
