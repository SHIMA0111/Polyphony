"use client"

import { useState } from "react"
import { Box, Button, Menu, Portal } from "@chakra-ui/react"
import { ChevronDown, Check } from "lucide-react"
import { useChangeMemberRole } from "../hooks/use-change-member-role"
import { RoleBadge } from "./RoleBadge"
import type { RoomRole } from "../types"

/**
 * Roles this endpoint may grant — never `master` (the server rejects
 * granting ownership via `PATCH /rooms/:roomId/members/:userId/role`; use
 * the dedicated ownership-transfer endpoint instead).
 */
const ASSIGNABLE_ROLES: RoomRole[] = ["reader", "guest", "member", "admin"]

interface RolePickerProps {
  roomId: string
  userId: string
  currentRole: RoomRole
}

/**
 * A `Menu.Root` letting an admin/master change another member's role to
 * one of the four non-owner roles, with the current role pre-highlighted
 * via a checkmark and a brief inline error if the mutation rejects (e.g. a
 * `409` when unexpectedly targeting the owner).
 *
 * `handleSelect` closes the menu synchronously on selection rather than
 * waiting for `onSuccess`, and bails out entirely while a change is already
 * `isPending` (the trigger button is also disabled for the same window) —
 * without this, a second selection fired before the first PATCH resolves
 * could have its response land first and get clobbered by the earlier
 * request's later-arriving response.
 */
export function RolePicker({ roomId, userId, currentRole }: RolePickerProps) {
  const [open, setOpen] = useState(false)
  const changeRoleMutation = useChangeMemberRole(roomId)

  const handleSelect = (role: RoomRole) => {
    if (changeRoleMutation.isPending) return
    if (role === currentRole) {
      setOpen(false)
      return
    }
    setOpen(false)
    changeRoleMutation.mutate({ userId, role })
  }

  return (
    <Box>
      <Menu.Root open={open} onOpenChange={(e) => setOpen(e.open)}>
        <Menu.Trigger asChild>
          <Button
            variant="outline"
            size="xs"
            gap={1}
            px={2}
            disabled={changeRoleMutation.isPending}
          >
            <RoleBadge role={currentRole} />
            <ChevronDown size={12} />
          </Button>
        </Menu.Trigger>
        <Portal>
          <Menu.Positioner>
            <Menu.Content minW="10rem">
              <Menu.ItemGroup>
                <Menu.ItemGroupLabel>Change role</Menu.ItemGroupLabel>
                {ASSIGNABLE_ROLES.map((role) => (
                  <Menu.Item
                    key={role}
                    value={role}
                    justifyContent="space-between"
                    onSelect={() => handleSelect(role)}
                  >
                    <RoleBadge role={role} variant="plain" />
                    {role === currentRole && <Check size={14} />}
                  </Menu.Item>
                ))}
              </Menu.ItemGroup>
            </Menu.Content>
          </Menu.Positioner>
        </Portal>
      </Menu.Root>
      {changeRoleMutation.isError && (
        <Box mt={1} fontSize="xs" color="fg.error" role="alert">
          {changeRoleMutation.error instanceof Error
            ? changeRoleMutation.error.message
            : "Failed to change role."}
        </Box>
      )}
    </Box>
  )
}
