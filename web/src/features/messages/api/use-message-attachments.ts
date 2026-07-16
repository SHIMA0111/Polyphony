"use client"

import { useQuery } from "@tanstack/react-query"
import { listAttachments } from "./list-attachments"
import type { AttachmentWithUrl } from "../types"

/**
 * The query key `useMessageAttachments` reads/writes, shared with
 * `use-chat-room.ts`'s send-with-attachments sequencing so a freshly-sent
 * message's attachments can be proactively seeded into this exact cache
 * entry (see that hook's `seedAttachmentsCache`) rather than this hook
 * having to wait on its own network round trip.
 */
export function messageAttachmentsQueryKey(roomId: string, messageId: string) {
  return ["rooms", roomId, "messages", messageId, "attachments"] as const
}

/**
 * Attachments linked to a single message, sourced from `GET
 * /api/proxy/rooms/:roomId/messages/:messageId/attachments`.
 *
 * Rendered by `MessageAttachments.tsx` for every rendered *persisted* human
 * message (`enabled` is `false` for a client-only optimistic entry, which has
 * no server-side attachments to look up yet). For a message just sent this
 * session, `useChatRoom`'s send-with-attachments sequencing has already
 * seeded this exact query key with the just-linked attachments, so this
 * resolves from cache instantly instead of showing a loading flash.
 */
export function useMessageAttachments(
  roomId: string,
  messageId: string,
  enabled = true,
) {
  return useQuery({
    queryKey: messageAttachmentsQueryKey(roomId, messageId),
    queryFn: async (): Promise<AttachmentWithUrl[]> => {
      const res = await listAttachments(roomId, messageId)
      return res.attachments
    },
    enabled,
  })
}
