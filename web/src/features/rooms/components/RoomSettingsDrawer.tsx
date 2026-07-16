"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import {
  Box,
  Button,
  Dialog,
  Drawer,
  Field,
  Flex,
  Heading,
  IconButton,
  Input,
  NativeSelect,
  Portal,
  Separator,
  Text,
  Textarea,
} from "@chakra-ui/react"
import { X } from "lucide-react"
import { toaster } from "@/components/ui/toaster"
import { formatModelMeta } from "@/features/messages/components/ModelSelector"
import { useModels } from "@/features/messages/hooks/use-models"
import { isOwnerRole, roleAtLeast } from "@/features/members/lib/roles"
import type { RoomRole } from "@/features/members/types"
import { useDeleteRoom } from "../hooks/use-delete-room"
import { useUpdateAIContextCutoff } from "../hooks/use-update-ai-context-cutoff"
import { useUpdateRoom } from "../hooks/use-update-room"
import { useUpdateRoomSettings } from "../hooks/use-update-room-settings"
import type { Room } from "../types"

/** `<option>` value meaning "use the deployment-wide default model". */
const GLOBAL_DEFAULT_VALUE = ""

interface RoomSettingsDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The already-loaded room this drawer edits. */
  room: Room
  /** The viewer's own role in `room` -- the single source of truth for every gate below. */
  role: RoomRole
}

/** Formats an ISO cutoff timestamp for display, or a fallback when unset. */
function formatCutoff(cutoffAt: string | null): string {
  if (!cutoffAt) return "No cutoff set"
  return new Date(cutoffAt).toLocaleString()
}

/**
 * The room settings drawer (`Drawer.Root`), opened from `ChatRoomHeader`'s
 * previously dead "more" button. Any member sees the room's name,
 * description, AI provider/model default, and AI context cutoff read-only.
 * `admin`/`master` additionally get editable controls for all four
 * (rename/description via `PUT /rooms/:roomId`, the AI default via
 * `PATCH /rooms/:roomId/settings`, the cutoff via
 * `PATCH /rooms/:roomId/ai-context-cutoff`); only `master` sees the
 * "Danger zone" delete-room action, gated behind a nested confirmation
 * dialog. Mirrors `MemberPanel`'s controlled `Drawer.Root` pattern and its
 * `room.role`-gated sections.
 */
export function RoomSettingsDrawer({
  open,
  onOpenChange,
  room,
  role,
}: RoomSettingsDrawerProps) {
  const router = useRouter()
  // `admin` and above may edit; only the current owner (`master`) may delete.
  const canManage = roleAtLeast(role, "admin")
  const canDelete = isOwnerRole(role)

  const modelsQuery = useModels()
  const models = modelsQuery.data ?? []

  const updateRoomMutation = useUpdateRoom(room.id)
  const updateSettingsMutation = useUpdateRoomSettings(room.id)
  const updateCutoffMutation = useUpdateAIContextCutoff(room.id)
  const deleteRoomMutation = useDeleteRoom()

  const [name, setName] = useState(room.name)
  const [description, setDescription] = useState(room.description)
  const [selectedModelId, setSelectedModelId] = useState(
    room.ai_model ?? GLOBAL_DEFAULT_VALUE,
  )
  const [cutoffInput, setCutoffInput] = useState("")
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  // Reconcile local edit state with the latest room data whenever the
  // drawer opens (or the underlying room data changes, e.g. after a save)
  // -- not on every render, so mid-edit keystrokes aren't clobbered by an
  // in-flight background refetch.
  useEffect(() => {
    if (!open) return
    setName(room.name)
    setDescription(room.description)
    setSelectedModelId(room.ai_model ?? GLOBAL_DEFAULT_VALUE)
    setCutoffInput("")
  }, [open, room.id, room.name, room.description, room.ai_model])

  const reportError = (title: string, err: unknown) => {
    toaster.create({
      type: "error",
      title,
      description: err instanceof Error ? err.message : "Please try again.",
    })
  }

  const handleSaveRoom = async () => {
    try {
      await updateRoomMutation.mutateAsync({ name, description })
    } catch (err) {
      reportError("Failed to update room", err)
    }
  }

  const handleSaveAISettings = async () => {
    const selectedModel = models.find((model) => model.id === selectedModelId)
    try {
      await updateSettingsMutation.mutateAsync({
        ai_provider: selectedModel?.provider ?? "",
        ai_model: selectedModel?.id ?? "",
      })
    } catch (err) {
      reportError("Failed to update AI default", err)
    }
  }

  const handleSaveCutoff = async (cutoffAt: string | null) => {
    try {
      await updateCutoffMutation.mutateAsync({ cutoff_at: cutoffAt })
    } catch (err) {
      reportError("Failed to update AI context cutoff", err)
    }
  }

  const handleConfirmDelete = async () => {
    try {
      await deleteRoomMutation.mutateAsync(room.id)
      setDeleteConfirmOpen(false)
      onOpenChange(false)
      router.push("/rooms")
    } catch (err) {
      reportError("Failed to delete room", err)
    }
  }

  return (
    <Drawer.Root open={open} onOpenChange={(e) => onOpenChange(e.open)}>
      <Portal>
        <Drawer.Backdrop />
        <Drawer.Positioner>
          <Drawer.Content>
            <Drawer.Header>
              <Drawer.Title>Room settings</Drawer.Title>
            </Drawer.Header>
            <Drawer.Body>
              <Flex direction="column" gap={6}>
                <Flex direction="column" gap={3}>
                  <Heading size="sm">Room details</Heading>
                  {canManage ? (
                    <>
                      <Field.Root>
                        <Field.Label>Room name</Field.Label>
                        <Input
                          aria-label="Room name"
                          value={name}
                          onChange={(e) => setName(e.target.value)}
                        />
                      </Field.Root>
                      <Field.Root>
                        <Field.Label>Description</Field.Label>
                        <Textarea
                          aria-label="Description"
                          rows={3}
                          value={description}
                          onChange={(e) => setDescription(e.target.value)}
                        />
                      </Field.Root>
                      <Button
                        size="sm"
                        colorPalette="blue"
                        alignSelf="flex-start"
                        disabled={!name.trim()}
                        loading={updateRoomMutation.isPending}
                        onClick={handleSaveRoom}
                      >
                        Save
                      </Button>
                    </>
                  ) : (
                    <>
                      <Box>
                        <Text fontSize="xs" color="fg.muted">
                          Room name
                        </Text>
                        <Text fontSize="sm">{room.name}</Text>
                      </Box>
                      <Box>
                        <Text fontSize="xs" color="fg.muted">
                          Description
                        </Text>
                        <Text fontSize="sm">
                          {room.description || "No description"}
                        </Text>
                      </Box>
                    </>
                  )}
                </Flex>

                <Separator />

                <Flex direction="column" gap={3}>
                  <Heading size="sm">AI default</Heading>
                  {canManage ? (
                    <>
                      <Field.Root>
                        <Field.Label>Model</Field.Label>
                        <NativeSelect.Root size="sm">
                          <NativeSelect.Field
                            aria-label="AI default model"
                            value={selectedModelId}
                            onChange={(e) =>
                              setSelectedModelId(e.currentTarget.value)
                            }
                          >
                            <option value={GLOBAL_DEFAULT_VALUE}>
                              Use global default
                            </option>
                            {models.map((model) => {
                              const meta = formatModelMeta(model)
                              return (
                                <option key={model.id} value={model.id}>
                                  {model.provider} - {model.name}
                                  {meta ? ` (${meta})` : ""}
                                </option>
                              )
                            })}
                          </NativeSelect.Field>
                          <NativeSelect.Indicator />
                        </NativeSelect.Root>
                      </Field.Root>
                      <Button
                        size="sm"
                        colorPalette="blue"
                        alignSelf="flex-start"
                        loading={updateSettingsMutation.isPending}
                        onClick={handleSaveAISettings}
                      >
                        Save
                      </Button>
                    </>
                  ) : (
                    <Box>
                      <Text fontSize="xs" color="fg.muted">
                        Default model
                      </Text>
                      <Text fontSize="sm">
                        {room.ai_model ?? "Use global default"}
                      </Text>
                    </Box>
                  )}
                </Flex>

                <Separator />

                <Flex direction="column" gap={3}>
                  <Heading size="sm">AI context cutoff</Heading>
                  <Text fontSize="xs" color="fg.muted">
                    Messages created before this point are excluded from AI
                    context.
                  </Text>
                  <Box>
                    <Text fontSize="xs" color="fg.muted">
                      Current cutoff
                    </Text>
                    <Text fontSize="sm">
                      {formatCutoff(room.ai_context_cutoff_at)}
                    </Text>
                  </Box>
                  {canManage && (
                    <Flex direction="column" gap={2}>
                      <Flex gap={2} wrap="wrap">
                        <Button
                          size="sm"
                          variant="outline"
                          loading={updateCutoffMutation.isPending}
                          onClick={() =>
                            handleSaveCutoff(new Date().toISOString())
                          }
                        >
                          Set cutoff to now
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          colorPalette="red"
                          disabled={!room.ai_context_cutoff_at}
                          loading={updateCutoffMutation.isPending}
                          onClick={() => handleSaveCutoff(null)}
                        >
                          Clear cutoff
                        </Button>
                      </Flex>
                      <Flex gap={2}>
                        <Input
                          aria-label="Cutoff date and time"
                          type="datetime-local"
                          size="sm"
                          value={cutoffInput}
                          onChange={(e) => setCutoffInput(e.target.value)}
                        />
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={!cutoffInput}
                          loading={updateCutoffMutation.isPending}
                          onClick={() =>
                            handleSaveCutoff(new Date(cutoffInput).toISOString())
                          }
                        >
                          Set
                        </Button>
                      </Flex>
                    </Flex>
                  )}
                </Flex>

                {canDelete && (
                  <>
                    <Separator />
                    <Flex direction="column" gap={3}>
                      <Heading size="sm" color="fg.error">
                        Danger zone
                      </Heading>
                      <Dialog.Root
                        open={deleteConfirmOpen}
                        // Nested inside this Drawer.Root, which is already a
                        // modal -- see `TransferOwnershipDialog`'s identical
                        // `modal={false}`/`trapFocus` note for why a second
                        // independent modal layer here otherwise breaks the
                        // outer drawer's own close/Escape handling.
                        modal={false}
                        trapFocus
                        onOpenChange={(e) => setDeleteConfirmOpen(e.open)}
                      >
                        <Dialog.Trigger asChild>
                          <Button
                            size="sm"
                            colorPalette="red"
                            alignSelf="flex-start"
                          >
                            Delete room
                          </Button>
                        </Dialog.Trigger>
                        <Portal>
                          <Dialog.Backdrop />
                          <Dialog.Positioner>
                            <Dialog.Content maxW="420px">
                              <Dialog.Header>
                                <Dialog.Title>Delete this room?</Dialog.Title>
                                <Dialog.Description color="fg.muted">
                                  This permanently deletes &ldquo;{room.name}
                                  &rdquo; and its message history. This action
                                  cannot be undone.
                                </Dialog.Description>
                              </Dialog.Header>
                              <Dialog.Footer>
                                <Button
                                  variant="outline"
                                  onClick={() => setDeleteConfirmOpen(false)}
                                >
                                  Cancel
                                </Button>
                                <Button
                                  colorPalette="red"
                                  loading={deleteRoomMutation.isPending}
                                  onClick={handleConfirmDelete}
                                >
                                  Yes, delete room
                                </Button>
                              </Dialog.Footer>
                              <Dialog.CloseTrigger />
                            </Dialog.Content>
                          </Dialog.Positioner>
                        </Portal>
                      </Dialog.Root>
                    </Flex>
                  </>
                )}
              </Flex>
            </Drawer.Body>
            <Drawer.Footer>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Close
              </Button>
            </Drawer.Footer>
            <Drawer.CloseTrigger asChild>
              <IconButton
                aria-label="Close room settings"
                variant="ghost"
                size="sm"
              >
                <X size={16} />
              </IconButton>
            </Drawer.CloseTrigger>
          </Drawer.Content>
        </Drawer.Positioner>
      </Portal>
    </Drawer.Root>
  )
}
