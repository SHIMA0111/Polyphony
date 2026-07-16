import { apiRequest } from "@/lib/http-client"
import type { Group } from "../types"

export interface CreateGroupInput {
  name: string
  description: string
}

/** Calls `POST /api/proxy/groups`. */
export function createGroup(input: CreateGroupInput): Promise<Group> {
  return apiRequest<Group>("/groups", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
