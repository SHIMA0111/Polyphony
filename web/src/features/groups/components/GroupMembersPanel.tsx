"use client"

import { Box, Flex, Skeleton, Text } from "@chakra-ui/react"
import { useGroupMembers } from "../hooks/use-group-members"
import { AddGroupMemberForm } from "./AddGroupMemberForm"
import { GroupMemberRow } from "./GroupMemberRow"

interface GroupMembersPanelProps {
  groupId: string
}

/**
 * Composes `AddGroupMemberForm` above a list of `GroupMemberRow`s sourced
 * from `useGroupMembers(groupId)`, with a loading skeleton and an empty
 * state. Rendered by `GroupDetail`.
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

      {membersQuery.isError && (
        <Text fontSize="sm" color="fg.error" role="alert">
          Failed to load members.
        </Text>
      )}
    </Flex>
  )
}
