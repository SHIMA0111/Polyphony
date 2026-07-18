"use client"

import { useState } from "react"
import Link from "next/link"
import { Box, Button, Flex, Menu, Portal, Text } from "@chakra-ui/react"
import { ChevronDown, Users } from "lucide-react"
import { useGroups } from "../hooks/use-groups"
import type { Group } from "../types"

interface GroupPickerProps {
  /**
   * The currently-selected group, owned by the caller (e.g. `InviteDialog`)
   * rather than this component — lifting the selection up keeps a single
   * source of truth so the trigger's label never goes stale across the
   * caller's own open/close or reset cycles.
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
 * to `/groups` instead of an unusable empty menu.
 */
export function GroupPicker({ selectedGroup, onSelect }: GroupPickerProps) {
  const [open, setOpen] = useState(false)
  const { data, isPending, isError, refetch } = useGroups()
  const groups = data?.groups ?? []

  const handleSelect = (group: Group) => {
    onSelect(group)
    setOpen(false)
  }

  if (isError) {
    return (
      <Flex direction="column" align="flex-start" gap={2}>
        <Text fontSize="sm" color="fg.error" role="alert">
          Failed to load groups.
        </Text>
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
