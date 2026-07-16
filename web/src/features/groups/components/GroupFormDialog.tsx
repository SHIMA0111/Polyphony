"use client"

import { useState } from "react"
import { Box, Button, Dialog, Field, Flex, Input, Portal, Textarea } from "@chakra-ui/react"
import { Pencil, Plus } from "lucide-react"
import { getErrorMessage } from "@/lib/get-error-message"
import { useCreateGroup } from "../hooks/use-create-group"
import { useUpdateGroup } from "../hooks/use-update-group"
import type { Group } from "../types"

interface GroupFormDialogProps {
  mode: "create" | "edit"
  /** Pre-fills `name`/`description` in edit mode; ignored in create mode. */
  initialGroup?: Group
}

/**
 * A `Dialog.Root` used for both creating a new personal group and
 * renaming/re-describing an existing one, selected via `mode`. In edit mode,
 * `initialGroup` pre-fills the `name`/`description` fields and the submit
 * button calls `useUpdateGroup(initialGroup.id)` instead of
 * `useCreateGroup()`.
 */
export function GroupFormDialog({ mode, initialGroup }: GroupFormDialogProps) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(initialGroup?.name ?? "")
  const [description, setDescription] = useState(initialGroup?.description ?? "")

  const createGroupMutation = useCreateGroup()
  const updateGroupMutation = useUpdateGroup(initialGroup?.id ?? "")
  const mutation = mode === "create" ? createGroupMutation : updateGroupMutation

  // Re-sync local field state whenever a fresh `initialGroup` arrives (e.g.
  // the group detail query refetches after some other edit), so opening the
  // dialog again doesn't show values from an earlier mount. Done during
  // render (React's "adjust state while rendering" pattern, same as
  // LoginForm/MessageInput) instead of a `useEffect`-that-calls-`setState`:
  // the `lastInitialGroup` marker makes this fire exactly once per new
  // `initialGroup` object, and the `useState` initializers above already
  // cover the mount-time case.
  const [lastInitialGroup, setLastInitialGroup] = useState(initialGroup)
  if (mode === "edit" && initialGroup && initialGroup !== lastInitialGroup) {
    setLastInitialGroup(initialGroup)
    setName(initialGroup.name)
    setDescription(initialGroup.description)
  }

  const resetState = () => {
    setName(mode === "edit" ? (initialGroup?.name ?? "") : "")
    setDescription(mode === "edit" ? (initialGroup?.description ?? "") : "")
    mutation.reset()
  }

  const handleSubmit = async () => {
    if (!name.trim()) return
    try {
      await mutation.mutateAsync({
        name: name.trim(),
        description: description.trim(),
      })
      setOpen(false)
      if (mode === "create") {
        setName("")
        setDescription("")
      }
    } catch {
      // Surfaced below via `mutation.isError`.
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(e) => {
        setOpen(e.open)
        if (!e.open) resetState()
      }}
    >
      <Dialog.Trigger asChild>
        {mode === "create" ? (
          <Button colorPalette="blue" gap={2}>
            <Plus size={16} />
            New group
          </Button>
        ) : (
          <Button variant="outline" size="sm" gap={2}>
            <Pencil size={14} />
            Edit
          </Button>
        )}
      </Dialog.Trigger>
      <Portal>
        <Dialog.Backdrop />
        <Dialog.Positioner>
          <Dialog.Content maxW="480px">
            <Dialog.Header>
              <Dialog.Title>
                {mode === "create" ? "Create a new group" : "Edit group"}
              </Dialog.Title>
              <Dialog.Description color="fg.muted">
                {mode === "create"
                  ? "Groups let you invite the same set of people to a room in one action."
                  : "Rename or re-describe this group."}
              </Dialog.Description>
            </Dialog.Header>
            <Dialog.Body>
              <Flex direction="column" gap={4}>
                <Field.Root>
                  <Field.Label>Name</Field.Label>
                  <Input
                    placeholder="e.g., Design Team"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </Field.Root>
                <Field.Root>
                  <Field.Label>Description</Field.Label>
                  <Textarea
                    placeholder="What is this group for?"
                    rows={3}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                  />
                </Field.Root>
                {mutation.isError && (
                  <Box fontSize="sm" color="fg.error" role="alert">
                    {getErrorMessage(
                      mutation.error,
                      `Failed to ${mode === "create" ? "create" : "update"} group.`,
                    )}
                  </Box>
                )}
              </Flex>
            </Dialog.Body>
            <Dialog.Footer>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                colorPalette="blue"
                disabled={!name.trim()}
                loading={mutation.isPending}
                onClick={handleSubmit}
              >
                {mode === "create" ? "Create group" : "Save changes"}
              </Button>
            </Dialog.Footer>
            <Dialog.CloseTrigger />
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
