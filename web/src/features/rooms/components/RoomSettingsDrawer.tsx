"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  Alert,
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
  Progress,
  Separator,
  Text,
  Textarea,
} from "@chakra-ui/react"
import { X } from "lucide-react"
import { toaster } from "@/components/ui/toaster"
import { formatModelMeta } from "@/features/messages/components/ModelSelector"
import { useModels } from "@/features/messages/hooks/use-models"
import { isOwnerRole, roleAtLeast } from "@/features/members/lib/roles"
import { formatDateTimeUtc } from "@/lib/format"
import type { RoomRole } from "@/features/members/types"
import { forkRoom } from "../api/fork-room"
import { useDeleteRoom } from "../hooks/use-delete-room"
import { useForkJob } from "../hooks/use-fork-job"
import { useUpdateAIContextCutoff } from "../hooks/use-update-ai-context-cutoff"
import { useUpdateRoom } from "../hooks/use-update-room"
import { useUpdateRoomSettings } from "../hooks/use-update-room-settings"
import type { ForkJob, Room } from "../types"

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

/**
 * Formats an ISO cutoff timestamp for display, or a fallback when unset.
 *
 * Uses the shared, UTC-pinned `formatDateTimeUtc` (`@/lib/format`) rather
 * than a bare `Date.prototype.toLocaleString()`: the latter reads the
 * runtime's own locale/timezone and so can render a different string on the
 * server than in the browser -- a latent React hydration mismatch.
 */
function formatCutoff(cutoffAt: string | null): string {
  if (!cutoffAt) return "No cutoff set"
  return formatDateTimeUtc(cutoffAt)
}

/**
 * The room settings drawer (`Drawer.Root`), opened from `ChatRoomHeader`'s
 * previously dead "more" button. Any member sees the room's name,
 * description, AI provider/model default, and AI context cutoff read-only.
 * `admin`/`master` additionally get editable controls for all four
 * (rename/description via `PUT /rooms/:roomId`, the AI default via
 * `PATCH /rooms/:roomId/settings`, the cutoff via
 * `PATCH /rooms/:roomId/ai-context-cutoff`), plus a "Fork room" action
 * (`POST /rooms/:roomId/fork`, Step 52); only `master` sees the
 * "Danger zone" delete-room action, gated behind a nested confirmation
 * dialog. Mirrors `MemberPanel`'s controlled `Drawer.Root` pattern and its
 * `room.role`-gated sections.
 *
 * The actual form/section content lives in {@link RoomSettingsDrawerBody},
 * remounted (via the `key` below) every time the drawer opens or the room
 * it's editing changes, so each editable field's local state
 * (name/description/selected model/cutoff input) always initializes fresh
 * from the current `room` -- without a reset-on-open effect syncing state
 * that was already seeded from props.
 *
 * The fork-job state (`forkJobId`/`forkRoomMutation`/`useForkJob`) is
 * deliberately owned *here*, above that `key` boundary, rather than inside
 * `RoomSettingsDrawerBody` -- a fork can run for a while (Step 32 copies
 * 1000 messages/batch), and this drawer is never unmounted by its parent
 * (`ChatRoom` always renders it, toggling only `open`; see this component's
 * own render). If the fork state instead lived in the remounted body, a
 * fork started, then the drawer closed (remounting the body on next open)
 * and reopened mid-copy, would silently lose its job id and `useForkJob`
 * poll -- reopening would show "Fork this room" again instead of the fork
 * that is still actually running server-side.
 */
export function RoomSettingsDrawer({
  open,
  onOpenChange,
  room,
  role,
}: RoomSettingsDrawerProps) {
  const queryClient = useQueryClient()

  const [forkJobId, setForkJobId] = useState<string | undefined>(undefined)
  const forkRoomMutation = useMutation({
    mutationFn: () => forkRoom(room.id),
    onSuccess: (data) => {
      // Seed the fork-job query cache with the response's initial
      // ("pending") job state so the progress view below renders
      // immediately, without waiting on `useForkJob`'s first poll.
      queryClient.setQueryData(
        ["rooms", room.id, "fork-jobs", data.job.id],
        data.job,
      )
      setForkJobId(data.job.id)
    },
  })
  const forkJobQuery = useForkJob(room.id, forkJobId)

  return (
    <Drawer.Root open={open} onOpenChange={(e) => onOpenChange(e.open)}>
      <Portal>
        <Drawer.Backdrop />
        <Drawer.Positioner>
          <Drawer.Content>
            <Drawer.Header>
              <Drawer.Title>Room settings</Drawer.Title>
            </Drawer.Header>
            <RoomSettingsDrawerBody
              key={open ? room.id : "closed"}
              room={room}
              role={role}
              onClose={() => onOpenChange(false)}
              forkJob={forkJobQuery.data}
              isForkPending={forkRoomMutation.isPending}
              onForkRoom={() => forkRoomMutation.mutateAsync()}
            />
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

interface RoomSettingsDrawerBodyProps {
  /** The already-loaded room this drawer edits. */
  room: Room
  /** The viewer's own role in `room` -- the single source of truth for every gate below. */
  role: RoomRole
  /** Closes the parent `Drawer.Root` (e.g. after a successful delete). */
  onClose: () => void
  /**
   * The fork job currently tracked for `room`, if any -- owned by
   * `RoomSettingsDrawer` above this component's own per-open `key` remount
   * boundary (see that component's docstring for why).
   */
  forkJob: ForkJob | undefined
  /** Whether a fork-trigger request is in flight -- lifted alongside `forkJob`. */
  isForkPending: boolean
  /** Triggers a new fork -- lifted alongside `forkJob`; rejects the same way the underlying mutation does. */
  onForkRoom: () => Promise<unknown>
}

/**
 * `RoomSettingsDrawer`'s body/footer content, split out solely so it can be
 * remounted by `key` on open/room-id change (see that component's
 * docstring) instead of syncing local edit state to `room` via an effect.
 * The fork-job state is the deliberate exception -- see `forkJob`'s own doc
 * comment above and `RoomSettingsDrawer`'s docstring.
 */
function RoomSettingsDrawerBody({
  room,
  role,
  onClose,
  forkJob,
  isForkPending,
  onForkRoom,
}: RoomSettingsDrawerBodyProps) {
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

  // Guards `handleSaveAISettings`: `useModels()` is empty until it resolves,
  // and a previously-saved model can also be absent from the catalog (e.g.
  // deprecated/removed upstream). Either way `models.find` below would miss
  // and silently submit `""`/`""` -- clearing the room's AI default instead
  // of leaving it untouched. "Use global default" is always a valid save.
  const canSaveAISettings =
    selectedModelId === GLOBAL_DEFAULT_VALUE ||
    models.some((model) => model.id === selectedModelId)

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
    if (!canSaveAISettings) return
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

  const handleForkRoom = async () => {
    try {
      await onForkRoom()
    } catch (err) {
      reportError("Failed to fork room", err)
    }
  }

  const handleConfirmDelete = async () => {
    try {
      await deleteRoomMutation.mutateAsync(room.id)
      setDeleteConfirmOpen(false)
      onClose()
      router.push("/rooms")
    } catch (err) {
      reportError("Failed to delete room", err)
    }
  }

  return (
    <>
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
                        disabled={!canSaveAISettings}
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

                {canManage && (
                  <>
                    <Separator />
                    <Flex direction="column" gap={3}>
                      <Heading size="sm">Fork room</Heading>
                      <Text fontSize="xs" color="fg.muted">
                        Copy every message in this room into a new,
                        independent room.
                      </Text>
                      {forkJob ? (
                        forkJob.status === "completed" ? (
                          <Flex direction="column" gap={2} align="flex-start">
                            <Text fontSize="sm" color="fg.success">
                              Fork complete -- the new room is ready.
                            </Text>
                            <Link href={`/rooms/${forkJob.new_room_id}`}>
                              <Button size="sm" colorPalette="blue">
                                Open forked room
                              </Button>
                            </Link>
                          </Flex>
                        ) : forkJob.status === "failed" ? (
                          <Flex direction="column" gap={2} align="flex-start">
                            <Alert.Root status="error">
                              <Alert.Indicator />
                              <Alert.Content>
                                <Alert.Title>Fork failed</Alert.Title>
                                <Alert.Description>
                                  {forkJob.error_message ??
                                    "Please try again."}
                                </Alert.Description>
                              </Alert.Content>
                            </Alert.Root>
                            <Button
                              size="sm"
                              variant="outline"
                              loading={isForkPending}
                              onClick={handleForkRoom}
                            >
                              Try again
                            </Button>
                          </Flex>
                        ) : (
                          <Flex direction="column" gap={2}>
                            <Progress.Root
                              value={
                                forkJob.total_messages > 0
                                  ? forkJob.copied_messages
                                  : null
                              }
                              max={
                                forkJob.total_messages > 0
                                  ? forkJob.total_messages
                                  : undefined
                              }
                            >
                              <Progress.Track>
                                <Progress.Range />
                              </Progress.Track>
                            </Progress.Root>
                            <Text fontSize="xs" color="fg.muted">
                              {forkJob.total_messages > 0
                                ? `Copying messages… ${forkJob.copied_messages} / ${forkJob.total_messages}`
                                : "Preparing…"}
                            </Text>
                          </Flex>
                        )
                      ) : (
                        <Button
                          size="sm"
                          colorPalette="blue"
                          alignSelf="flex-start"
                          loading={isForkPending}
                          onClick={handleForkRoom}
                        >
                          Fork this room
                        </Button>
                      )}
                    </Flex>
                  </>
                )}

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
        <Button variant="outline" onClick={onClose}>
          Close
        </Button>
      </Drawer.Footer>
    </>
  )
}
