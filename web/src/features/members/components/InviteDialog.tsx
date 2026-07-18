"use client"

import { useState } from "react"
import {
  Box,
  Button,
  Dialog,
  Field,
  Flex,
  Input,
  NativeSelect,
  Portal,
  Separator,
  Text,
} from "@chakra-ui/react"
import { Copy, Link2, UserPlus } from "lucide-react"
import { toaster } from "@/components/ui/toaster"
import { GroupPicker } from "@/features/groups/components/GroupPicker"
import { useBatchInviteByGroup } from "@/features/groups/hooks/use-batch-invite-by-group"
import type { BatchInviteByGroupResult, Group } from "@/features/groups/types"
import { useCreateInvitation } from "../hooks/use-create-invitation"
import type { Invitation, RoomRole } from "../types"

/** How long the copy button shows its "copied" state before reverting — mirrors `CodeBlock.tsx`. */
const COPY_FEEDBACK_MS = 2000

/** Which of the two independent invite flows currently has a mutation in flight. */
type PendingAction = "username" | "link" | null

/** Roles an inviter may offer — never `master` (see `RolePicker`'s same restriction). */
const INVITABLE_ROLES: RoomRole[] = ["reader", "guest", "member", "admin"]

interface InviteDialogProps {
  roomId: string
}

/**
 * Inline summary rendered after a successful batch-invite-by-group call:
 * the invited count, followed by one `"{username}: {reason}"` row per
 * skipped member (already a room member, already has a pending invitation,
 * etc.) — a single RBAC failure surfaces as one top-level inline error
 * instead of reaching this component at all (see
 * `batchInviteByGroupMutation.isError` below).
 */
function BatchInviteSummary({ result }: { result: BatchInviteByGroupResult }) {
  return (
    <Flex direction="column" gap={1}>
      <Text fontSize="xs" color="fg.muted">
        Invited {result.invited.length} member(s).
      </Text>
      {result.skipped.length > 0 && (
        <Flex direction="column" gap={0.5}>
          {result.skipped.map((skip) => (
            <Text key={skip.user_id} fontSize="xs" color="fg.muted">
              {skip.username}: {skip.reason}
            </Text>
          ))}
        </Flex>
      )}
    </Flex>
  )
}

/**
 * A `Dialog.Root` with three independent invite flows for a room's
 * admin/master: inviting an exact, existing username (single-use),
 * generating a reusable, copyable link invitation, or batch-inviting every
 * member of one of the caller's own personal groups (Step 46) at a chosen
 * role in a single call. All three sections share the same
 * `canManageMembers(room.role)` gate: `MemberPanel` (this dialog's only
 * caller) already only renders `InviteDialog` at all for admin/master
 * viewers, so no section re-checks the gate independently.
 */
export function InviteDialog({ roomId }: InviteDialogProps) {
  const [open, setOpen] = useState(false)
  const [username, setUsername] = useState("")
  const [usernameRole, setUsernameRole] = useState<RoomRole>("member")
  const [linkRole, setLinkRole] = useState<RoomRole>("member")
  const [linkInvitation, setLinkInvitation] = useState<Invitation | null>(null)
  const [copied, setCopied] = useState(false)
  const [pendingAction, setPendingAction] = useState<PendingAction>(null)
  const [selectedGroup, setSelectedGroup] = useState<Group | null>(null)
  const [groupRole, setGroupRole] = useState<RoomRole>("member")
  const [groupExpiresInHours, setGroupExpiresInHours] = useState("")
  const [groupExpiresError, setGroupExpiresError] = useState<string | null>(null)

  const createInvitationMutation = useCreateInvitation(roomId)
  const batchInviteByGroupMutation = useBatchInviteByGroup(roomId)

  const resetState = () => {
    setUsername("")
    setUsernameRole("member")
    setLinkRole("member")
    setLinkInvitation(null)
    setCopied(false)
    setPendingAction(null)
    setSelectedGroup(null)
    setGroupRole("member")
    setGroupExpiresInHours("")
    setGroupExpiresError(null)
    createInvitationMutation.reset()
    batchInviteByGroupMutation.reset()
  }

  const handleInviteByUsername = async () => {
    if (!username.trim()) return
    setPendingAction("username")
    try {
      await createInvitationMutation.mutateAsync({
        invitee_username: username.trim(),
        role: usernameRole,
      })
      setUsername("")
    } catch {
      // Surfaced below via `createInvitationMutation.isError`.
    }
  }

  const handleGenerateLink = async () => {
    setPendingAction("link")
    try {
      const invitation = await createInvitationMutation.mutateAsync({
        role: linkRole,
      })
      setLinkInvitation(invitation)
      setCopied(false)
    } catch {
      // Surfaced below via `createInvitationMutation.isError`.
    }
  }

  const inviteUrl = linkInvitation
    ? `${window.location.origin}/invite/${linkInvitation.invite_code}`
    : ""

  const handleCopy = async () => {
    if (!inviteUrl) return
    try {
      await navigator.clipboard.writeText(inviteUrl)
      setCopied(true)
      setTimeout(() => setCopied(false), COPY_FEEDBACK_MS)
    } catch {
      toaster.create({
        type: "error",
        title: "Failed to copy invite link",
        description: "Please try again.",
      })
    }
  }

  const handleBatchInviteByGroup = async () => {
    if (!selectedGroup) return
    const trimmedExpiresInHours = groupExpiresInHours.trim()
    let expiresInHours: number | undefined
    if (trimmedExpiresInHours) {
      const parsed = Number(trimmedExpiresInHours)
      // Reject fractional/out-of-range values client-side: the server
      // otherwise responds with a generic decode error for fractional
      // input, and `Number(...)` on garbage silently yields `NaN`, which
      // `JSON.stringify` drops and the server treats as "use the default".
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 720) {
        setGroupExpiresError("Expiration must be a whole number of hours between 1 and 720.")
        return
      }
      expiresInHours = parsed
    }
    setGroupExpiresError(null)
    try {
      await batchInviteByGroupMutation.mutateAsync({
        group_id: selectedGroup.id,
        role: groupRole,
        ...(expiresInHours !== undefined && { expires_in_hours: expiresInHours }),
      })
    } catch {
      // Surfaced below via `batchInviteByGroupMutation.isError`.
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
        <Button size="sm" colorPalette="blue" gap={2}>
          <UserPlus size={16} />
          Invite
        </Button>
      </Dialog.Trigger>
      <Portal>
        <Dialog.Backdrop />
        <Dialog.Positioner>
          <Dialog.Content maxW="480px">
            <Dialog.Header>
              <Dialog.Title>Invite to this room</Dialog.Title>
              <Dialog.Description color="fg.muted">
                Invite an exact username, or share a reusable link
              </Dialog.Description>
            </Dialog.Header>
            <Dialog.Body>
              <Flex direction="column" gap={4}>
                <Flex direction="column" gap={3}>
                  <Text fontSize="sm" fontWeight="medium">
                    Invite by username
                  </Text>
                  <Field.Root>
                    <Field.Label>Username</Field.Label>
                    <Input
                      placeholder="exact username"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                    />
                  </Field.Root>
                  <Field.Root>
                    <Field.Label>Role</Field.Label>
                    <NativeSelect.Root size="sm">
                      <NativeSelect.Field
                        value={usernameRole}
                        onChange={(e) =>
                          setUsernameRole(e.currentTarget.value as RoomRole)
                        }
                      >
                        {INVITABLE_ROLES.map((role) => (
                          <option key={role} value={role}>
                            {role}
                          </option>
                        ))}
                      </NativeSelect.Field>
                      <NativeSelect.Indicator />
                    </NativeSelect.Root>
                  </Field.Root>
                  <Button
                    size="sm"
                    variant="outline"
                    alignSelf="flex-start"
                    disabled={!username.trim()}
                    loading={
                      createInvitationMutation.isPending &&
                      pendingAction === "username"
                    }
                    onClick={handleInviteByUsername}
                  >
                    Send invitation
                  </Button>
                  {createInvitationMutation.isSuccess &&
                    createInvitationMutation.data?.invitee_id && (
                      <Box fontSize="xs" color="fg.success" role="status">
                        Invitation sent.
                      </Box>
                    )}
                </Flex>

                <Separator />

                <Flex direction="column" gap={3}>
                  <Text fontSize="sm" fontWeight="medium">
                    Generate a shareable link
                  </Text>
                  <Field.Root>
                    <Field.Label>Role</Field.Label>
                    <NativeSelect.Root size="sm">
                      <NativeSelect.Field
                        value={linkRole}
                        onChange={(e) =>
                          setLinkRole(e.currentTarget.value as RoomRole)
                        }
                      >
                        {INVITABLE_ROLES.map((role) => (
                          <option key={role} value={role}>
                            {role}
                          </option>
                        ))}
                      </NativeSelect.Field>
                      <NativeSelect.Indicator />
                    </NativeSelect.Root>
                  </Field.Root>
                  <Button
                    size="sm"
                    variant="outline"
                    alignSelf="flex-start"
                    gap={2}
                    loading={
                      createInvitationMutation.isPending && pendingAction === "link"
                    }
                    onClick={handleGenerateLink}
                  >
                    <Link2 size={14} />
                    Generate link
                  </Button>
                  {linkInvitation && (
                    <Flex gap={2}>
                      <Input value={inviteUrl} readOnly fontSize="xs" />
                      <Button size="sm" variant="outline" onClick={handleCopy} gap={1}>
                        <Copy size={14} />
                        {copied ? "Copied" : "Copy"}
                      </Button>
                    </Flex>
                  )}
                </Flex>

                {createInvitationMutation.isError && (
                  <Box fontSize="sm" color="fg.error" role="alert">
                    {createInvitationMutation.error instanceof Error
                      ? createInvitationMutation.error.message
                      : "Failed to create invitation."}
                  </Box>
                )}

                <Separator />

                <Flex direction="column" gap={3}>
                  <Text fontSize="sm" fontWeight="medium">
                    Invite a group
                  </Text>
                  <Field.Root>
                    <Field.Label>Group</Field.Label>
                    <GroupPicker selectedGroup={selectedGroup} onSelect={setSelectedGroup} />
                  </Field.Root>
                  <Field.Root>
                    <Field.Label>Role</Field.Label>
                    <NativeSelect.Root size="sm">
                      <NativeSelect.Field
                        value={groupRole}
                        onChange={(e) =>
                          setGroupRole(e.currentTarget.value as RoomRole)
                        }
                      >
                        {INVITABLE_ROLES.map((role) => (
                          <option key={role} value={role}>
                            {role}
                          </option>
                        ))}
                      </NativeSelect.Field>
                      <NativeSelect.Indicator />
                    </NativeSelect.Root>
                  </Field.Root>
                  <Field.Root>
                    <Field.Label>Expires in (hours, optional)</Field.Label>
                    <Input
                      type="number"
                      placeholder="e.g., 168"
                      min={1}
                      max={720}
                      step={1}
                      value={groupExpiresInHours}
                      onChange={(e) => {
                        setGroupExpiresInHours(e.target.value)
                        setGroupExpiresError(null)
                      }}
                    />
                  </Field.Root>
                  <Button
                    size="sm"
                    variant="outline"
                    alignSelf="flex-start"
                    gap={2}
                    disabled={!selectedGroup}
                    loading={batchInviteByGroupMutation.isPending}
                    onClick={handleBatchInviteByGroup}
                  >
                    <UserPlus size={16} />
                    Invite group
                  </Button>
                  {batchInviteByGroupMutation.isSuccess &&
                    batchInviteByGroupMutation.data && (
                      <Box fontSize="xs" color="fg.muted" role="status">
                        <BatchInviteSummary result={batchInviteByGroupMutation.data} />
                      </Box>
                    )}
                  {groupExpiresError && (
                    <Box fontSize="sm" color="fg.error" role="alert">
                      {groupExpiresError}
                    </Box>
                  )}
                  {batchInviteByGroupMutation.isError && (
                    <Box fontSize="sm" color="fg.error" role="alert">
                      {batchInviteByGroupMutation.error instanceof Error
                        ? batchInviteByGroupMutation.error.message
                        : "Failed to invite group."}
                    </Box>
                  )}
                </Flex>
              </Flex>
            </Dialog.Body>
            <Dialog.Footer>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Close
              </Button>
            </Dialog.Footer>
            <Dialog.CloseTrigger />
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
