"use client"

import { useRouter } from "next/navigation"
import { Button, Card, Flex, Heading, Spinner, Text } from "@chakra-ui/react"
import { Check, X } from "lucide-react"
import { useInvitationByCode } from "../hooks/use-invitation-by-code"
import { useAcceptInvitation } from "../hooks/use-accept-invitation"
import { useRejectInvitation } from "../hooks/use-reject-invitation"
import { RoleBadge } from "./RoleBadge"

interface InviteAcceptViewProps {
  code: string
}

/**
 * The `/invite/[code]` landing page's client-side content: previews an
 * invitation by its shareable `invite_code` (role + expiry only — the
 * by-code endpoint doesn't return a room name, see
 * `docs/tasks/step37.md`'s Out of scope) and lets the signed-in viewer
 * accept it (and reject it, for a username-targeted invitation only — a
 * link invitation has no meaningful "reject" per Step 21, so navigating
 * away is the only alternative offered for those).
 *
 * Every failure mode this page can hit (404 on the by-code fetch; 403/409/410
 * on accept/reject) renders a short inline status message in place of the
 * action buttons rather than letting the error surface as an unhandled
 * exception.
 */
export function InviteAcceptView({ code }: InviteAcceptViewProps) {
  const router = useRouter()
  const invitationQuery = useInvitationByCode(code)
  const acceptMutation = useAcceptInvitation()
  const rejectMutation = useRejectInvitation()

  const handleAccept = async () => {
    if (!invitationQuery.data) return
    try {
      const result = await acceptMutation.mutateAsync(invitationQuery.data.id)
      router.push(`/rooms/${result.room_id}`)
    } catch {
      // Surfaced below via `acceptMutation.isError`.
    }
  }

  const handleReject = () => {
    if (!invitationQuery.data) return
    rejectMutation.mutate(invitationQuery.data.id)
  }

  return (
    <Flex minH="100vh" align="center" justify="center" bg="bg.subtle" p={4}>
      <Card.Root w="full" maxW="420px" shadow="lg">
        <Card.Header textAlign="center">
          <Heading size="xl">Room invitation</Heading>
        </Card.Header>
        <Card.Body>
          {invitationQuery.isPending ? (
            <Flex justify="center" py={8}>
              <Spinner size="lg" colorPalette="blue" />
            </Flex>
          ) : invitationQuery.isError ? (
            <Text textAlign="center" color="fg.error">
              This invitation link is invalid or has expired.
            </Text>
          ) : (
            <Flex direction="column" align="center" gap={4}>
              <Flex align="center" gap={2}>
                <Text color="fg.muted">Role offered:</Text>
                <RoleBadge role={invitationQuery.data.role} />
              </Flex>
              <Text fontSize="sm" color="fg.muted">
                Expires{" "}
                {new Date(invitationQuery.data.expires_at).toLocaleDateString(
                  "en-US",
                  { month: "short", day: "numeric", timeZone: "UTC" },
                )}
              </Text>

              {rejectMutation.isSuccess ? (
                <Text color="fg.muted">Invitation rejected.</Text>
              ) : acceptMutation.isError ? (
                <Text color="fg.error" role="alert">
                  {acceptMutation.error instanceof Error
                    ? acceptMutation.error.message
                    : "Could not accept this invitation."}
                </Text>
              ) : rejectMutation.isError ? (
                <Text color="fg.error" role="alert">
                  {rejectMutation.error instanceof Error
                    ? rejectMutation.error.message
                    : "Could not reject this invitation."}
                </Text>
              ) : (
                <Flex direction="column" align="center" gap={3} w="full">
                  {invitationQuery.data.invitee_id === null && (
                    <Text fontSize="xs" color="fg.muted" textAlign="center">
                      Anyone with this link can join — you can simply
                      navigate away instead of joining.
                    </Text>
                  )}
                  <Flex gap={2} w="full">
                    <Button
                      flex={1}
                      colorPalette="blue"
                      gap={2}
                      loading={acceptMutation.isPending}
                      onClick={handleAccept}
                    >
                      <Check size={16} />
                      Accept
                    </Button>
                    {invitationQuery.data.invitee_id !== null && (
                      <Button
                        flex={1}
                        variant="outline"
                        colorPalette="red"
                        gap={2}
                        loading={rejectMutation.isPending}
                        onClick={handleReject}
                      >
                        <X size={16} />
                        Reject
                      </Button>
                    )}
                  </Flex>
                </Flex>
              )}
            </Flex>
          )}
        </Card.Body>
      </Card.Root>
    </Flex>
  )
}
