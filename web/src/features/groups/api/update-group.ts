import { apiRequest } from "@/lib/http-client"
import type { Group } from "../types"

export interface UpdateGroupInput {
  name: string
  description: string
}

/**
 * Calls `PUT /api/proxy/groups/:groupId` — Step 40's group-update endpoint is
 * a full-object replace over `PUT`, unlike this codebase's other mutations
 * (`POST`/`PATCH`/`DELETE`), so this calls `apiRequest` directly with
 * `method: "PUT"` rather than a bespoke `httpClient` verb method.
 */
export function updateGroup(
  groupId: string,
  input: UpdateGroupInput,
): Promise<Group> {
  return apiRequest<Group>(`/groups/${groupId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}
