"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { createGroup, type CreateGroupInput } from "../api/create-group"

/** Creates a new personal group and invalidates the `["groups"]` list on success. */
export function useCreateGroup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: CreateGroupInput) => createGroup(input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["groups"] })
    },
  })
}
