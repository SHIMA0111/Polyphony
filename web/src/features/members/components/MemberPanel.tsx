"use client"

import { Box, Drawer, Flex, Heading, IconButton, Portal, Skeleton } from "@chakra-ui/react"
import { X } from "lucide-react"
import { useMembers } from "../hooks/use-members"
import { canManageMembers } from "../lib/roles"
import { InviteDialog } from "./InviteDialog"
import { MemberListItem } from "./MemberListItem"
import { TransferOwnershipDialog } from "./TransferOwnershipDialog"
import type { Room } from "@/features/rooms/types"

interface MemberPanelProps {
  room: Room
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * The full member-list side panel (`Drawer.Root`), opened from
 * `MemberAvatarStack`. `room.role` (the viewer's own role, already loaded
 * on `Room` — see Step 37's Scope) is the single source of truth for every
 * gate in this panel: the `InviteDialog` trigger only renders for
 * `admin`/`master` viewers, and the "Transfer ownership" entry point only
 * for the current owner (`master`). Per-row gating (role picker, leave
 * action) is delegated to `MemberListItem`, which receives the same
 * `room.role` rather than re-deriving it by scanning the member list.
 */
export function MemberPanel({ room, open, onOpenChange }: MemberPanelProps) {
  const membersQuery = useMembers(room.id)
  const members = membersQuery.data?.members ?? []

  return (
    <Drawer.Root open={open} onOpenChange={(e) => onOpenChange(e.open)}>
      <Portal>
        <Drawer.Backdrop />
        <Drawer.Positioner>
          <Drawer.Content>
            <Drawer.Header>
              <Flex align="center" justify="space-between" flex={1} gap={3}>
                <Drawer.Title>Members</Drawer.Title>
                <Flex gap={2}>
                  {canManageMembers(room.role) && (
                    <InviteDialog roomId={room.id} />
                  )}
                </Flex>
              </Flex>
            </Drawer.Header>
            <Drawer.Body>
              {membersQuery.isPending ? (
                <Flex direction="column" gap={3}>
                  {[1, 2, 3].map((i) => (
                    <Skeleton key={i} h={10} rounded="md" />
                  ))}
                </Flex>
              ) : members.length === 0 ? (
                <Box color="fg.muted" fontSize="sm">
                  No members found.
                </Box>
              ) : (
                <Flex direction="column">
                  {members.map((member) => (
                    <MemberListItem
                      key={member.id}
                      roomId={room.id}
                      member={member}
                      viewerRole={room.role}
                    />
                  ))}
                </Flex>
              )}

              {room.role === "master" && (
                <Box mt={6}>
                  <Heading size="sm" mb={2}>
                    Ownership
                  </Heading>
                  <TransferOwnershipDialog room={room} />
                </Box>
              )}
            </Drawer.Body>
            <Drawer.CloseTrigger asChild>
              <IconButton aria-label="Close member panel" variant="ghost" size="sm">
                <X size={16} />
              </IconButton>
            </Drawer.CloseTrigger>
          </Drawer.Content>
        </Drawer.Positioner>
      </Portal>
    </Drawer.Root>
  )
}
