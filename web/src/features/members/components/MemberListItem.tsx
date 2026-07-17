"use client"

import { Avatar, Badge, Box, Button, Flex, Text } from "@chakra-ui/react"
import { LogOut } from "lucide-react"
import { useCurrentUser } from "@/features/auth/hooks/use-current-user"
import { useLeaveRoom } from "../hooks/use-leave-room"
import { canManageMembers, isOwnerRole } from "@/lib/roles"
import { RoleBadge } from "./RoleBadge"
import { RolePicker } from "./RolePicker"
import type { Member, RoomRole } from "../types"

interface MemberListItemProps {
  roomId: string
  member: Member
  /** The viewer's own role in this room (`Room.role`) — the single source
   * of truth for whether this row's management controls render. */
  viewerRole: RoomRole
}

/**
 * A single row in `MemberPanel`'s member list: avatar, identity, role
 * badge, and — gated by the viewer's own role — either a `RolePicker` (for
 * another non-owner member, when the viewer can manage members) or, on the
 * viewer's own row, a "You" tag plus an inline "Leave" action (unless the
 * viewer owns the room, since the owner must transfer ownership first).
 */
export function MemberListItem({ roomId, member, viewerRole }: MemberListItemProps) {
  // Deliberately `useCurrentUser` (`GET /users/me`, the local `users.id`),
  // not `useSession().data?.identity.id` (Kratos's own identity UUID) --
  // under `AUTH_MODE=kratos` the two are never equal, and `member.user_id`
  // is the local id, so comparing against `identity.id` here would never
  // recognize the viewer's own row. See `CurrentUser`'s doc comment in
  // `@/features/auth/types.ts`.
  const { data: currentUser } = useCurrentUser()
  const isSelf = currentUser?.id === member.user_id
  const isOwnerRow = isOwnerRole(member.role)
  const leaveRoomMutation = useLeaveRoom(roomId, member.user_id)

  // `username` is `""` on responses from non-JOINed lookups (e.g. right
  // after a role-change mutation, before the member-list query refetches) —
  // fall back to the raw user id so the row never renders a blank identity.
  const displayName = member.username || member.user_id

  return (
    <Flex
      align="center"
      gap={3}
      py={2}
      data-testid={`member-row-${member.user_id}`}
    >
      <Avatar.Root size="sm">
        <Avatar.Fallback name={member.username || undefined} />
      </Avatar.Root>

      <Box flex={1} minW={0}>
        <Text fontSize="sm" fontWeight="medium" truncate>
          {displayName}
        </Text>
        {member.username && (
          <Text fontSize="xs" color="fg.muted" fontFamily="mono" truncate>
            {member.user_id}
          </Text>
        )}
      </Box>

      {isSelf ? (
        <Flex align="center" gap={2}>
          <Badge size="sm" variant="outline" colorPalette="gray">
            You
          </Badge>
          <RoleBadge role={member.role} />
          {!isOwnerRow && (
            <Button
              variant="ghost"
              size="xs"
              colorPalette="red"
              gap={1}
              loading={leaveRoomMutation.isPending}
              onClick={() => leaveRoomMutation.mutate()}
            >
              <LogOut size={14} />
              Leave
            </Button>
          )}
        </Flex>
      ) : canManageMembers(viewerRole) && !isOwnerRow ? (
        <RolePicker roomId={roomId} userId={member.user_id} currentRole={member.role} />
      ) : (
        <RoleBadge role={member.role} />
      )}
    </Flex>
  )
}
