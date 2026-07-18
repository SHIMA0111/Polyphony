"use client"

import { useState } from "react"
import { Box, Button, Field, Flex, Input } from "@chakra-ui/react"
import { UserPlus } from "lucide-react"
import { useAddGroupMember } from "../hooks/use-add-group-member"

interface AddGroupMemberFormProps {
  groupId: string
}

/**
 * A small inline form adding an existing user to a group by exact username.
 * Surfaces the server's `404` (unknown username) / `409` (already a member)
 * responses inline, mirroring `GroupFormDialog`'s error-display pattern.
 */
export function AddGroupMemberForm({ groupId }: AddGroupMemberFormProps) {
  const [username, setUsername] = useState("")
  const addGroupMemberMutation = useAddGroupMember(groupId)

  const handleSubmit = async () => {
    // The Enter-key path (`onKeyDown` below) bypasses the submit button's
    // own `disabled`/`loading` gating, so a held-down or repeated Enter can
    // otherwise fire a duplicate `POST` before the first response returns,
    // surfacing a misleading `409` for the second request.
    if (addGroupMemberMutation.isPending) return
    if (!username.trim()) return
    try {
      await addGroupMemberMutation.mutateAsync(username.trim())
      setUsername("")
    } catch {
      // Surfaced below via `addGroupMemberMutation.isError`.
    }
  }

  return (
    <Box>
      <Flex gap={2} align="flex-end">
        <Field.Root flex={1}>
          <Field.Label>Add member</Field.Label>
          <Input
            placeholder="exact username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                handleSubmit()
              }
            }}
          />
        </Field.Root>
        <Button
          variant="outline"
          gap={2}
          disabled={!username.trim()}
          loading={addGroupMemberMutation.isPending}
          onClick={handleSubmit}
        >
          <UserPlus size={16} />
          Add
        </Button>
      </Flex>
      {addGroupMemberMutation.isError && (
        <Box mt={2} fontSize="sm" color="fg.error" role="alert">
          {addGroupMemberMutation.error instanceof Error
            ? addGroupMemberMutation.error.message
            : "Failed to add member."}
        </Box>
      )}
    </Box>
  )
}
