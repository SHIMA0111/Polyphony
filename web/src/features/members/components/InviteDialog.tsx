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
import { getErrorMessage } from "@/lib/get-error-message"
import { GroupPicker } from "@/features/groups/components/GroupPicker"
import { useBatchInviteByGroup } from "@/features/groups/hooks/use-batch-invite-by-group"
import type { BatchInviteByGroupResult, Group } from "@/features/groups/types"
import { useCreateInvitation } from "../hooks/use-create-invitation"
import type { Invitation, RoomRole } from "../types"

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
  const [selectedGroup, setSelectedGroup] = useState<Group | null>(null)
  const [groupRole, setGroupRole] = useState<RoomRole>("member")
  const [groupExpiresInHours, setGroupExpiresInHours] = useState("")

  const createInvitationMutation = useCreateInvitation(roomId)
  const batchInviteByGroupMutation = useBatchInviteByGroup(roomId)

  const resetState = () => {
    setUsername("")
    setUsernameRole("member")
    setLinkRole("member")
    setLinkInvitation(null)
    setCopied(false)
    setSelectedGroup(null)
    setGroupRole("member")
    setGroupExpiresInHours("")
    createInvitationMutation.reset()
    batchInviteByGroupMutation.reset()
  }

  const handleInviteByUsername = async () => {
    if (!username.trim()) return
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
    await navigator.clipboard.writeText(inviteUrl)
    setCopied(true)
  }

  const handleBatchInviteByGroup = async () => {
    if (!selectedGroup) return
    const expiresInHours = groupExpiresInHours.trim()
      ? Number(groupExpiresInHours)
      : undefined
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
                    loading={createInvitationMutation.isPending}
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
                    loading={createInvitationMutation.isPending}
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
                    {getErrorMessage(createInvitationMutation.error, "Failed to create invitation.")}
                  </Box>
                )}

                <Separator />

                <Flex direction="column" gap={3}>
                  <Text fontSize="sm" fontWeight="medium">
                    Invite a group
                  </Text>
                  <Field.Root>
                    <Field.Label>Group</Field.Label>
                    <GroupPicker onSelect={setSelectedGroup} />
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
                      value={groupExpiresInHours}
                      onChange={(e) => setGroupExpiresInHours(e.target.value)}
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
                  {batchInviteByGroupMutation.isError && (
                    <Box fontSize="sm" color="fg.error" role="alert">
                      {getErrorMessage(batchInviteByGroupMutation.error, "Failed to invite group.")}
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
