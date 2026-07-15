"use client"

import { useState } from "react"
import { Avatar, AvatarGroup, Button } from "@chakra-ui/react"
import { useMembers } from "../hooks/use-members"
import { MemberPanel } from "./MemberPanel"
import type { Room } from "@/features/rooms/types"

/** Maximum number of avatars shown before collapsing the rest into a "+N" badge. */
const MAX_VISIBLE = 4

interface MemberAvatarStackProps {
  room: Room
}

/**
 * A compact header widget rendering up to {@link MAX_VISIBLE} overlapping
 * member avatars (a "+N" badge for the rest), wrapped in a clickable
 * ghost button that opens `MemberPanel`.
 *
 * Accepts the already-fetched `room` (not just a bare `roomId`) so it and
 * `MemberPanel` can read the viewer's own `room.role` for gating without a
 * second round-trip.
 */
export function MemberAvatarStack({ room }: MemberAvatarStackProps) {
  const [panelOpen, setPanelOpen] = useState(false)
  const membersQuery = useMembers(room.id)
  const members = membersQuery.data?.members ?? []
  const visibleMembers = members.slice(0, MAX_VISIBLE)
  const overflowCount = members.length - visibleMembers.length

  if (membersQuery.isPending) {
    return (
      <AvatarGroup gap={0} spaceX="-3" size="sm">
        <Avatar.Root opacity={0.4}>
          <Avatar.Fallback />
        </Avatar.Root>
        <Avatar.Root opacity={0.4}>
          <Avatar.Fallback />
        </Avatar.Root>
      </AvatarGroup>
    )
  }

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        p={0}
        h="auto"
        rounded="full"
        aria-label="Room members"
        onClick={() => setPanelOpen(true)}
      >
        <AvatarGroup gap={0} spaceX="-3" size="sm">
          {visibleMembers.map((member) => (
            <Avatar.Root key={member.id}>
              <Avatar.Fallback name={member.username || undefined} />
            </Avatar.Root>
          ))}
          {overflowCount > 0 && (
            <Avatar.Root variant="solid">
              <Avatar.Fallback>+{overflowCount}</Avatar.Fallback>
            </Avatar.Root>
          )}
        </AvatarGroup>
      </Button>

      <MemberPanel room={room} open={panelOpen} onOpenChange={setPanelOpen} />
    </>
  )
}
