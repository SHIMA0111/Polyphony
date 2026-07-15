"use client"

import { useState } from "react"
import { Button, Dialog, Field, Flex, Input, Portal, Textarea } from "@chakra-ui/react"
import { Plus } from "lucide-react"
import { useCreateRoom } from "@/features/rooms/hooks/use-create-room"

export function CreateRoomForm() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const { mutateAsync: createRoom, isPending } = useCreateRoom()

  const handleSubmit = async () => {
    if (!name.trim()) return

    try {
      await createRoom({ name, description })
      setName("")
      setDescription("")
      setOpen(false)
    } catch {
      // TODO: show error toast
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={(e) => setOpen(e.open)}>
      <Dialog.Trigger asChild>
        <Button colorPalette="blue" size="lg" gap={2}>
          <Plus size={16} />
          New Room
        </Button>
      </Dialog.Trigger>
      <Portal>
        <Dialog.Backdrop />
        <Dialog.Positioner>
          <Dialog.Content maxW="500px">
            <Dialog.Header>
              <Dialog.Title>Create a new room</Dialog.Title>
              <Dialog.Description color="fg.muted">
                Set up a collaborative space for your team to chat with AI
              </Dialog.Description>
            </Dialog.Header>
            <Dialog.Body>
              <Flex direction="column" gap={4}>
                <Field.Root>
                  <Field.Label>Room name</Field.Label>
                  <Input
                    placeholder="e.g., Product Strategy"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </Field.Root>
                <Field.Root>
                  <Field.Label>Description</Field.Label>
                  <Textarea
                    placeholder="What is this room for?"
                    rows={3}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                  />
                </Field.Root>
              </Flex>
            </Dialog.Body>
            <Dialog.Footer>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                colorPalette="blue"
                onClick={handleSubmit}
                loading={isPending}
              >
                Create room
              </Button>
            </Dialog.Footer>
            <Dialog.CloseTrigger />
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
