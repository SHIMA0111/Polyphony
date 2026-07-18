import { apiRequest } from "@/lib/http-client"
import type { UploadTicket } from "../types"

/**
 * Calls `POST /api/proxy/rooms/:roomId/attachments/upload-url`, requesting a
 * presigned S3 upload ticket for a not-yet-uploaded attachment. The caller
 * (`use-attachment-staging.ts`) uploads the raw file bytes to
 * `ticket.upload_url` itself, via `../lib/upload-attachment.ts`.
 */
export function requestUploadUrl(
  roomId: string,
  mimeType: string,
  sizeBytes: number,
): Promise<UploadTicket> {
  return apiRequest<UploadTicket>(`/rooms/${roomId}/attachments/upload-url`, {
    method: "POST",
    body: JSON.stringify({ mime_type: mimeType, size_bytes: sizeBytes }),
  })
}
