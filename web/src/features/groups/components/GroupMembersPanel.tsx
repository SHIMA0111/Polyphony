"use client"

import { Box, Button, Flex, Skeleton, Text } from "@chakra-ui/react"
import { getErrorMessage } from "@/lib/get-error-message"
import { useGroupMembers } from "../hooks/use-group-members"
import { AddGroupMemberForm } from "./AddGroupMemberForm"
import { GroupMemberRow } from "./GroupMemberRow"

interface GroupMembersPanelProps {
  groupId: string
}

/**
 * Composes `AddGroupMemberForm` above a list of `GroupMemberRow`s sourced
 * from `useGroupMembers(groupId)`, with a loading skeleton, a retryable
 * error state, and an empty state — checked in that order (matching
 * `InvitationsInbox`'s isPending -> isError -> empty -> list branching) so
 * a failed load can no longer render "No members yet." *and* the error text
 * together (the empty-state check used to run unconditionally, gated only
 * on `members.length === 0`, which a failed query's `undefined` data also
 * satisfies). Rendered by `GroupDetail`.
 */
export function GroupMembersPanel({ groupId }: GroupMembersPanelProps) {
  const membersQuery = useGroupMembers(groupId)
  const members = membersQuery.data?.members ?? []

  return (
    <Flex direction="column" gap={4}>
      <AddGroupMemberForm groupId={groupId} />

      {membersQuery.isPending ? (
        <Flex direction="column" gap={3}>
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} h={10} rounded="md" />
          ))}
        </Flex>
      ) : membersQuery.isError ? (
        <Flex direction="column" gap={2} align="flex-start">
          <Text fontSize="sm" color="fg.error" role="alert">
            {getErrorMessage(membersQuery.error, "Failed to load members.")}
          </Text>
          <Button size="xs" variant="outline" onClick={() => membersQuery.refetch()}>
            Retry
          </Button>
        </Flex>
      ) : members.length === 0 ? (
        <Box color="fg.muted" fontSize="sm">
          No members yet.
        </Box>
      ) : (
        <Flex direction="column">
          {members.map((member) => (
            <GroupMemberRow key={member.id} groupId={groupId} member={member} />
          ))}
        </Flex>
      )}
    </Flex>
  )
}
