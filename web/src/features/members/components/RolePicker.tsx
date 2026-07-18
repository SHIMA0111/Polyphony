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
 */
export function RolePicker({ roomId, userId, currentRole }: RolePickerProps) {
  const [open, setOpen] = useState(false)
  const changeRoleMutation = useChangeMemberRole(roomId)

  const handleSelect = (role: RoomRole) => {
    if (role === currentRole) {
      setOpen(false)
      return
    }
    changeRoleMutation.mutate(
      { userId, role },
      { onSuccess: () => setOpen(false) },
    )
  }

  return (
    <Box>
      <Menu.Root open={open} onOpenChange={(e) => setOpen(e.open)}>
        <Menu.Trigger asChild>
          <Button variant="outline" size="xs" gap={1} px={2}>
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
