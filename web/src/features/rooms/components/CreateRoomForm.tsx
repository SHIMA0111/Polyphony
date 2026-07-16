"use client"

import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Button, Dialog, Field, Flex, Input, Portal, Textarea } from "@chakra-ui/react"
import { Plus } from "lucide-react"
import { toaster } from "@/components/ui/toaster"
import { getErrorMessage } from "@/lib/get-error-message"
import { useCreateRoom } from "@/features/rooms/hooks/use-create-room"
import {
  createRoomSchema,
  type CreateRoomFormOutput,
  type CreateRoomFormValues,
} from "@/features/rooms/utils/schemas"

export function CreateRoomForm() {
  const [open, setOpen] = useState(false)
  const { mutateAsync: createRoom, isPending } = useCreateRoom()
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateRoomFormValues, unknown, CreateRoomFormOutput>({
    resolver: zodResolver(createRoomSchema),
    defaultValues: { name: "", description: "" },
  })

  const onSubmit = handleSubmit(async (values) => {
    try {
      await createRoom(values)
      reset()
      setOpen(false)
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Failed to create room",
        description: getErrorMessage(err, "Please try again."),
      })
    }
  })

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(e) => {
        setOpen(e.open)
        if (!e.open) reset()
      }}
    >
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
            <form onSubmit={onSubmit} noValidate>
              <Dialog.Header>
                <Dialog.Title>Create a new room</Dialog.Title>
                <Dialog.Description color="fg.muted">
                  Set up a collaborative space for your team to chat with AI
                </Dialog.Description>
              </Dialog.Header>
              <Dialog.Body>
                <Flex direction="column" gap={4}>
                  <Field.Root invalid={!!errors.name}>
                    <Field.Label>Room name</Field.Label>
                    <Input
                      placeholder="e.g., Product Strategy"
                      {...register("name")}
                    />
                    {errors.name && (
                      <Field.ErrorText role="alert">
                        {errors.name.message}
                      </Field.ErrorText>
                    )}
                  </Field.Root>
                  <Field.Root invalid={!!errors.description}>
                    <Field.Label>Description</Field.Label>
                    <Textarea
                      placeholder="What is this room for?"
                      rows={3}
                      {...register("description")}
                    />
                    {errors.description && (
                      <Field.ErrorText role="alert">
                        {errors.description.message}
                      </Field.ErrorText>
                    )}
                  </Field.Root>
                </Flex>
              </Dialog.Body>
              <Dialog.Footer>
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => setOpen(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" colorPalette="blue" loading={isPending}>
                  Create room
                </Button>
              </Dialog.Footer>
            </form>
            <Dialog.CloseTrigger />
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
