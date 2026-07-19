import { apiRequest } from "@/lib/http-client"
import type { AttachmentResponse } from "../types"

/**
 * Calls `POST /api/proxy/rooms/:roomId/messages/:messageId/attachments`,
 * linking a previously-uploaded attachment (already resolved via
 * `requestUploadUrl` + `../lib/upload-attachment.ts`) to a persisted
 * message.
 */
export function attachToMessage(
  roomId: string,
  messageId: string,
  attachmentId: string,
): Promise<AttachmentResponse> {
  return apiRequest<AttachmentResponse>(
    `/rooms/${roomId}/messages/${messageId}/attachments`,
    {
      method: "POST",
      body: JSON.stringify({ attachment_id: attachmentId }),
    },
  )
}
