"use client"

import { useRouter } from "next/navigation"
import { Box, Button, Flex, Text } from "@chakra-ui/react"
import { Check, X } from "lucide-react"
import { useMyInvitations } from "../hooks/use-my-invitations"
import { useAcceptInvitation } from "../hooks/use-accept-invitation"
import { useRejectInvitation } from "../hooks/use-reject-invitation"
import { RoleBadge } from "./RoleBadge"

/**
 * Lists the caller's own pending invitations across every room
 * (`useMyInvitations`), with inline Accept/Reject actions. Rendered inside
 * `InvitationsBellButton`'s `Popover.Root`.
 *
 * `Invitation` doesn't carry a room name (Step 21's `InvitationResponse`
 * only has `room_id` — see `docs/tasks/step37.md`'s Out of scope), so each
 * row identifies the room by its id rather than inventing a server change.
 */
export function InvitationsInbox() {
  const router = useRouter()
  const invitationsQuery = useMyInvitations()
  const acceptMutation = useAcceptInvitation()
  const rejectMutation = useRejectInvitation()

  const invitations = invitationsQuery.data?.invitations ?? []

  const handleAccept = async (invitationId: string) => {
    try {
      const result = await acceptMutation.mutateAsync(invitationId)
      router.push(`/rooms/${result.room_id}`)
    } catch {
      // Surfaced via `acceptMutation.isError` below.
    }
  }

  if (invitationsQuery.isPending) {
    return (
      <Text fontSize="sm" color="fg.muted" p={2}>
        Loading invitations...
      </Text>
    )
  }

  if (invitations.length === 0) {
    return (
      <Text fontSize="sm" color="fg.muted" p={2}>
        No pending invitations
      </Text>
    )
  }

  return (
    <Flex direction="column" gap={2}>
      {invitations.map((invitation) => (
        <Flex
          key={invitation.id}
          direction="column"
          gap={2}
          p={2}
          rounded="md"
          borderWidth="1px"
          borderColor="border.subtle"
        >
          <Flex align="center" justify="space-between" gap={2}>
            <Box minW={0}>
              <Text fontSize="xs" color="fg.muted" fontFamily="mono" truncate>
                Room {invitation.room_id}
              </Text>
              <Text fontSize="xs" color="fg.muted">
                Expires{" "}
                {new Date(invitation.expires_at).toLocaleDateString("en-US", {
                  month: "short",
                  day: "numeric",
                  timeZone: "UTC",
                })}
              </Text>
            </Box>
            <RoleBadge role={invitation.role} />
          </Flex>
          <Flex gap={2}>
            <Button
              size="xs"
              colorPalette="blue"
              gap={1}
              flex={1}
              loading={acceptMutation.isPending}
              onClick={() => handleAccept(invitation.id)}
            >
              <Check size={12} />
              Accept
            </Button>
            {invitation.invitee_id && (
              <Button
                size="xs"
                variant="outline"
                colorPalette="red"
                gap={1}
                flex={1}
                loading={rejectMutation.isPending}
                onClick={() => rejectMutation.mutate(invitation.id)}
              >
                <X size={12} />
                Reject
              </Button>
            )}
          </Flex>
        </Flex>
      ))}
      {(acceptMutation.isError || rejectMutation.isError) && (
        <Box fontSize="xs" color="fg.error" role="alert">
          {(acceptMutation.error ?? rejectMutation.error) instanceof Error
            ? ((acceptMutation.error ?? rejectMutation.error) as Error).message
            : "Something went wrong."}
        </Box>
      )}
    </Flex>
  )
}
