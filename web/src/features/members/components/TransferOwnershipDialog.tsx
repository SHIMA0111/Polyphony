"use client"

import { useState } from "react"
import { Avatar, Box, Button, Dialog, Flex, Portal, Text } from "@chakra-ui/react"
import { Crown } from "lucide-react"
import { useMembers } from "../hooks/use-members"
import { useTransferOwnership } from "../hooks/use-transfer-ownership"
import type { Room } from "@/features/rooms/types"

interface TransferOwnershipDialogProps {
  room: Room
}

/**
 * A `Dialog.Root` letting the current owner (`room.role === "master"`)
 * transfer ownership to another existing, non-owner member — a
 * button-row list of selectable candidates (reusing the
 * `bg={isSelected ? "bg.muted" : "transparent"}` row pattern already used
 * in `ModelSelector.tsx`), a confirm button, and a warning that the caller
 * becomes an admin once the transfer completes.
 */
export function TransferOwnershipDialog({ room }: TransferOwnershipDialogProps) {
  const [open, setOpen] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null)
  const membersQuery = useMembers(room.id)
  const transferMutation = useTransferOwnership(room.id)

  const candidates = (membersQuery.data?.members ?? []).filter(
    (member) => member.role !== "master",
  )

  const handleConfirm = async () => {
    if (!selectedUserId) return
    try {
      await transferMutation.mutateAsync(selectedUserId)
      setOpen(false)
      setSelectedUserId(null)
    } catch {
      // Surfaced below via `transferMutation.isError`.
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(e) => {
        setOpen(e.open)
        if (!e.open) {
          setSelectedUserId(null)
          transferMutation.reset()
        }
      }}
    >
      <Dialog.Trigger asChild>
        <Button size="sm" variant="outline" gap={2}>
          <Crown size={16} />
          Transfer ownership
        </Button>
      </Dialog.Trigger>
      <Portal>
        <Dialog.Backdrop />
        <Dialog.Positioner>
          <Dialog.Content maxW="420px">
            <Dialog.Header>
              <Dialog.Title>Transfer ownership</Dialog.Title>
              <Dialog.Description color="fg.muted">
                You will become an admin after transferring ownership.
              </Dialog.Description>
            </Dialog.Header>
            <Dialog.Body>
              {candidates.length === 0 ? (
                <Text fontSize="sm" color="fg.muted">
                  No other members to transfer ownership to.
                </Text>
              ) : (
                <Flex direction="column" gap={1}>
                  {candidates.map((member) => {
                    const isSelected = member.user_id === selectedUserId
                    return (
                      <Box
                        key={member.id}
                        as="button"
                        display="flex"
                        alignItems="center"
                        gap={3}
                        w="full"
                        rounded="md"
                        px={3}
                        py={2}
                        textAlign="left"
                        cursor="pointer"
                        bg={isSelected ? "bg.muted" : "transparent"}
                        _hover={{ bg: "bg.muted" }}
                        transition="backgrounds"
                        onClick={() => setSelectedUserId(member.user_id)}
                      >
                        <Avatar.Root size="xs">
                          <Avatar.Fallback name={member.username || undefined} />
                        </Avatar.Root>
                        <Text fontSize="sm" fontWeight="medium">
                          {member.username || member.user_id}
                        </Text>
                      </Box>
                    )
                  })}
                </Flex>
              )}
              {transferMutation.isError && (
                <Box mt={3} fontSize="sm" color="fg.error" role="alert">
                  {transferMutation.error instanceof Error
                    ? transferMutation.error.message
                    : "Failed to transfer ownership."}
                </Box>
              )}
            </Dialog.Body>
            <Dialog.Footer>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                colorPalette="orange"
                disabled={!selectedUserId}
                loading={transferMutation.isPending}
                onClick={handleConfirm}
              >
                Confirm transfer
              </Button>
            </Dialog.Footer>
            <Dialog.CloseTrigger />
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
