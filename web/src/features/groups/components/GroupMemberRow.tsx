"use client"

import { Avatar, Box, Button, Flex, Text } from "@chakra-ui/react"
import { UserMinus } from "lucide-react"
import { formatUtcDate } from "@/lib/format"
import { getErrorMessage } from "@/lib/get-error-message"
import { useRemoveGroupMember } from "../hooks/use-remove-group-member"
import type { GroupMember } from "../types"

interface GroupMemberRowProps {
  groupId: string
  member: GroupMember
}

/**
 * A single row in `GroupMembersPanel`'s member list. Unlike a room's
 * `MemberListItem`, `member.username` here is always resolved (Step 40's
 * `GroupMemberWithUsername` join) so this never falls back to the raw
 * `user_id`.
 */
export function GroupMemberRow({ groupId, member }: GroupMemberRowProps) {
  const removeGroupMemberMutation = useRemoveGroupMember(groupId)

  return (
    <Flex
      direction="column"
      gap={1}
      py={2}
      data-testid={`group-member-row-${member.user_id}`}
    >
      <Flex align="center" gap={3}>
        <Avatar.Root size="sm">
          <Avatar.Fallback name={member.username} />
        </Avatar.Root>

        <Box flex={1} minW={0}>
          <Text fontSize="sm" fontWeight="medium" truncate>
            {member.username}
          </Text>
          <Text fontSize="xs" color="fg.muted">
            Added {formatUtcDate(member.added_at)}
          </Text>
        </Box>

        <Button
          variant="ghost"
          size="xs"
          colorPalette="red"
          gap={1}
          loading={removeGroupMemberMutation.isPending}
          onClick={() => removeGroupMemberMutation.mutate(member.user_id)}
        >
          <UserMinus size={14} />
          Remove
        </Button>
      </Flex>

      {removeGroupMemberMutation.isError && (
        <Box fontSize="xs" color="fg.error" role="alert">
          {getErrorMessage(removeGroupMemberMutation.error, "Failed to remove member.")}
        </Box>
      )}
    </Flex>
  )
}
