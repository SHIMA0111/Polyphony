"use client"

import Link from "next/link"
import { useParams, usePathname } from "next/navigation"
import { Avatar, Box, Button, Flex, Heading, Menu, Portal } from "@chakra-ui/react"
import { LogOut, Pen, Users } from "lucide-react"
import { useLogout } from "@/features/auth/hooks/use-logout"
import { RoomRail } from "@/features/rooms/components/RoomRail"
import { InvitationsBellButton } from "@/features/members/components/InvitationsBellButton"
import { BalanceBadge } from "@/features/billing/components/BalanceBadge"

/**
 * Persistent shell for every route under the `(main)` route group
 * (`/rooms` and `/rooms/[roomId]`).
 *
 * Renders a top bar (logo mark, "Polyphony" heading, the `BalanceBadge`
 * token-balance widget, and the avatar `Menu.Root` with Groups/Logout —
 * moved here from `RoomList` so it mounts once instead of once per page)
 * above a two-region body: the
 * always-mounted `RoomRail` room-list rail and a content pane wrapping
 * `children` (`RoomList` at `/rooms`, `ChatRoom` at `/rooms/[roomId]`).
 * Navigating between `(main)` routes only swaps `children` via Next.js
 * client-side transitions — this layout, the top bar, and `RoomRail`'s
 * `useRooms()` query never remount or refetch.
 *
 * Below the `md` breakpoint the rail and the content pane collapse into a
 * single visible region. For room routes (`/rooms`, `/rooms/[roomId]`) which
 * region is chosen is driven purely by whether a `roomId` route param is
 * present (read via `useParams()`, which reflects whichever `(main)` page is
 * rendered beneath this layout) — the same component tree renders both
 * breakpoints, there is no separate mobile-only branch. Every other `(main)`
 * route (`/groups`, `/billing/*`, `/invite/*`, ...) has no rail to collapse
 * into, so those always render `children` at the base breakpoint and hide
 * `RoomRail` there instead.
 */
export default function MainLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const params = useParams<{ roomId?: string }>()
  const pathname = usePathname()
  const isRoomRoute = pathname === "/rooms" || pathname.startsWith("/rooms/")
  const hasActiveRoom = typeof params.roomId === "string"
  // At base breakpoint: room routes toggle between the rail and the content
  // pane depending on whether a room is active; every other route has no
  // rail to show, so it always renders the content pane.
  const showRailAtBase = isRoomRoute && !hasActiveRoom
  const showContentAtBase = !isRoomRoute || hasActiveRoom
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

          <Flex align="center" gap={3}>
            <InvitationsBellButton />
            {/* Persistent balance widget, immediately left of the avatar
                menu — its own slot, distinct from any other same-wave
                top-bar insertion anchored between the logo and this menu. */}
            <BalanceBadge />

            <Menu.Root>
              <Menu.Trigger asChild>
                <Button
                  aria-label="Account menu"
                  variant="ghost"
                  rounded="full"
                  p={0}
                  h={9}
                  w={9}
                >
                  <Avatar.Root size="sm" colorPalette="blue">
                    <Avatar.Fallback name="User" />
                  </Avatar.Root>
                </Button>
              </Menu.Trigger>
              <Portal>
                <Menu.Positioner>
                  <Menu.Content w="56">
                    <Menu.Item value="groups" gap={2} asChild>
                      <Link href="/groups">
                        <Users size={16} />
                        Groups
                      </Link>
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
        </Flex>
      </Box>

      {/* Rail + content pane row: collapses to a single visible region
          below `md`. Room routes toggle on `hasActiveRoom`; every other
          route always shows the content pane and hides the rail. */}
      <Flex flex={1} minH={0}>
        <RoomRail
          display={{ base: showRailAtBase ? "flex" : "none", md: "flex" }}
        />
        <Box
          flex={1}
          minW={0}
          overflowY="auto"
          display={{ base: showContentAtBase ? "flex" : "none", md: "flex" }}
        >
          {children}
        </Box>
      </Flex>
    </Flex>
  )
}
