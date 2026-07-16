"use client"

import { useState } from "react"
import { Box, Button, Card, Dialog, Flex, Heading, Portal, Skeleton, Text } from "@chakra-ui/react"
import { Trash2 } from "lucide-react"
import { useGroup } from "../hooks/use-group"
import { useDeleteGroup } from "../hooks/use-delete-group"
import { GroupFormDialog } from "./GroupFormDialog"
import { GroupMembersPanel } from "./GroupMembersPanel"

interface GroupDetailProps {
  groupId: string
}

/**
 * The `/groups/[groupId]` page content: the group's name/description (with
 * an "Edit" entry point opening `GroupFormDialog` in edit mode), a
 * destructive "Delete group" action gated behind a confirmation dialog
 * (mirroring the confirm-then-mutate pattern already used for other
 * destructive actions in this codebase, e.g. `TransferOwnershipDialog`), and
 * `GroupMembersPanel`. Loading/not-found states are driven by
 * `useGroup(groupId)`.
 */
export function GroupDetail({ groupId }: GroupDetailProps) {
  const groupQuery = useGroup(groupId)
  const deleteGroupMutation = useDeleteGroup()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const handleDelete = async () => {
    try {
      await deleteGroupMutation.mutateAsync(groupId)
      setConfirmOpen(false)
    } catch {
      // Surfaced below via `deleteGroupMutation.isError`.
    }
  }

  if (groupQuery.isPending) {
    return (
      <Box h="100%" bg="bg">
        <Box as="main" maxW="4xl" mx="auto" px={4} py={8}>
          <Skeleton h={8} w="240px" mb={4} />
          <Skeleton h={4} w="360px" />
        </Box>
      </Box>
    )
  }

  if (groupQuery.isError || !groupQuery.data) {
    return (
      <Box h="100%" bg="bg">
        <Box as="main" maxW="4xl" mx="auto" px={4} py={8}>
          <Heading size="md">Group not found</Heading>
          <Text color="fg.muted" mt={2}>
            This group may have been deleted, or you don&apos;t have access to it.
          </Text>
        </Box>
      </Box>
    )
  }

  const group = groupQuery.data

  return (
    <Box h="100%" bg="bg">
      <Box as="main" maxW="4xl" mx="auto" px={4} py={8}>
        <Flex align="flex-start" justify="space-between" mb={2} gap={4}>
          <Box>
            <Heading size="3xl" fontWeight="bold" letterSpacing="tight">
              {group.name}
            </Heading>
            <Text color="fg.muted" mt={1}>
              {group.description || "No description"}
            </Text>
          </Box>
          <Flex gap={2} flexShrink={0}>
            <GroupFormDialog mode="edit" initialGroup={group} />

            <Dialog.Root
              open={confirmOpen}
              onOpenChange={(e) => {
                setConfirmOpen(e.open)
                if (!e.open) deleteGroupMutation.reset()
              }}
            >
              <Dialog.Trigger asChild>
                <Button variant="outline" size="sm" colorPalette="red" gap={2}>
                  <Trash2 size={14} />
                  Delete group
                </Button>
              </Dialog.Trigger>
              <Portal>
                <Dialog.Backdrop />
                <Dialog.Positioner>
                  <Dialog.Content maxW="420px">
                    <Dialog.Header>
                      <Dialog.Title>Delete this group?</Dialog.Title>
                      <Dialog.Description color="fg.muted">
                        This permanently deletes &ldquo;{group.name}&rdquo; and its
                        membership list. This cannot be undone.
                      </Dialog.Description>
                    </Dialog.Header>
                    <Dialog.Body>
                      {deleteGroupMutation.isError && (
                        <Box fontSize="sm" color="fg.error" role="alert">
                          {deleteGroupMutation.error instanceof Error
                            ? deleteGroupMutation.error.message
                            : "Failed to delete group."}
                        </Box>
                      )}
                    </Dialog.Body>
                    <Dialog.Footer>
                      <Button variant="outline" onClick={() => setConfirmOpen(false)}>
                        Cancel
                      </Button>
                      <Button
                        colorPalette="red"
                        loading={deleteGroupMutation.isPending}
                        onClick={handleDelete}
                      >
                        Delete group
                      </Button>
                    </Dialog.Footer>
                    <Dialog.CloseTrigger />
                  </Dialog.Content>
                </Dialog.Positioner>
              </Portal>
            </Dialog.Root>
          </Flex>
        </Flex>

        <Card.Root mt={8}>
          <Card.Header>
            <Card.Title>Members</Card.Title>
          </Card.Header>
          <Card.Body>
            <GroupMembersPanel groupId={groupId} />
          </Card.Body>
        </Card.Root>
      </Box>
    </Box>
  )
}
