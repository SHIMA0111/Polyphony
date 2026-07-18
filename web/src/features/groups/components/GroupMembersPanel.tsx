"use client"

import { Box, Button, Flex, Skeleton, Text } from "@chakra-ui/react"
import { useGroupMembers } from "../hooks/use-group-members"
import { AddGroupMemberForm } from "./AddGroupMemberForm"
import { GroupMemberRow } from "./GroupMemberRow"

interface GroupMembersPanelProps {
  groupId: string
}

/**
 * Composes `AddGroupMemberForm` above a list of `GroupMemberRow`s sourced
 * from `useGroupMembers(groupId)`, with a loading skeleton, an error state
 * (with retry), and an empty state — in that order, matching
 * `InvitationsInbox`'s `isPending` -> `isError` -> empty -> list precedence
 * so a failed load never also shows "No members yet." underneath the error.
 * Rendered by `GroupDetail`.
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
        <Flex direction="column" align="flex-start" gap={2}>
          <Text fontSize="sm" color="fg.error" role="alert">
            Failed to load members.
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
