import { apiRequest } from "@/lib/http-client"
import type { AttachmentWithUrl } from "../types"

/** Raw response envelope from `GET /rooms/:roomId/messages/:messageId/attachments`. */
export interface AttachmentListResponse {
  attachments: AttachmentWithUrl[]
}

/**
 * Calls `GET /api/proxy/rooms/:roomId/messages/:messageId/attachments`,
 * returning every attachment linked to `messageId`, each with a freshly
 * presigned `view_url`.
 */
export function listAttachments(
  roomId: string,
  messageId: string,
): Promise<AttachmentListResponse> {
  return apiRequest<AttachmentListResponse>(
    `/rooms/${roomId}/messages/${messageId}/attachments`,
  )
}
