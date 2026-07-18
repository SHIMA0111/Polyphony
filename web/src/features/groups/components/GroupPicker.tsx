"use client"

import { useState } from "react"
import Link from "next/link"
import { Box, Button, Flex, Menu, Portal, Text } from "@chakra-ui/react"
import { ChevronDown, Users } from "lucide-react"
import { getErrorMessage } from "@/lib/get-error-message"
import { useGroups } from "../hooks/use-groups"
import type { Group } from "../types"

interface GroupPickerProps {
  /**
   * The currently selected group, or `null` if none is selected yet.
   * Owned by the caller rather than this component's own state:
   * `InviteDialog` (this component's only caller) already tracks its own
   * `selectedGroup` state to build the batch-invite request body, and a
   * second, independent copy of that selection living here went stale
   * across a dialog close/reopen — `InviteDialog`'s `resetState()` reset
   * its own state but had no way to reach into this component's internal
   * state, so the trigger label kept showing the group picked the
   * *previous* time the dialog was open.
   */
  selectedGroup: Group | null
  /** Called with the selected group when the caller picks one from the menu. */
  onSelect: (group: Group) => void
}

/**
 * A `Menu.Root` listing the caller's own personal groups (`useGroups()`),
 * mirroring the button-row-with-`bg={isSelected ? "bg.muted" : "transparent"}`
 * pattern already used in `ModelSelector.tsx`/`TransferOwnershipDialog.tsx`.
 * When the caller has no groups yet, renders an inline empty state linking
 * to `/groups` instead of an unusable empty menu; when the groups query
 * itself fails, renders a compact inline error (checked before the
 * empty-state check, since a failed query and a genuinely empty list are
 * different situations) with a retry action instead.
 */
export function GroupPicker({ selectedGroup, onSelect }: GroupPickerProps) {
  const [open, setOpen] = useState(false)
  const { data, isPending, isError, error, refetch } = useGroups()
  const groups = data?.groups ?? []

  const handleSelect = (group: Group) => {
    onSelect(group)
    setOpen(false)
  }

  if (!isPending && isError) {
    return (
      <Flex align="center" gap={2} fontSize="sm" color="fg.error">
        <Text role="alert">{getErrorMessage(error, "Failed to load groups.")}</Text>
        <Button size="xs" variant="outline" onClick={() => refetch()}>
          Retry
        </Button>
      </Flex>
    )
  }

  if (!isPending && groups.length === 0) {
    return (
      <Box fontSize="sm" color="fg.muted">
        No groups yet —{" "}
        <Link href="/groups">
          <Text as="span" color="blue.500" fontWeight="medium" _hover={{ textDecoration: "underline" }}>
            create one
          </Text>
        </Link>
      </Box>
    )
  }

  return (
    <Menu.Root open={open} onOpenChange={(e) => setOpen(e.open)}>
      <Menu.Trigger asChild>
        <Button variant="outline" size="sm" gap={2} justifyContent="space-between">
          <Box display="flex" alignItems="center" gap={2}>
            <Users size={14} />
            {selectedGroup ? selectedGroup.name : "Select a group"}
          </Box>
          <ChevronDown size={12} />
        </Button>
      </Menu.Trigger>
      <Portal>
        <Menu.Positioner>
          <Menu.Content minW="12rem">
            {groups.map((group) => {
              const isSelected = group.id === selectedGroup?.id
              return (
                <Menu.Item
                  key={group.id}
                  value={group.id}
                  bg={isSelected ? "bg.muted" : "transparent"}
                  onSelect={() => handleSelect(group)}
                >
                  {group.name}
                </Menu.Item>
              )
            })}
          </Menu.Content>
        </Menu.Positioner>
      </Portal>
    </Menu.Root>
  )
}
