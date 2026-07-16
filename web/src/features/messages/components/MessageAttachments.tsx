"use client"

import { useState } from "react"
import { Flex, Image } from "@chakra-ui/react"
import { useMessageAttachments } from "@/features/messages/api/use-message-attachments"
import type { Message } from "@/features/messages/types"
import { AttachmentLightbox } from "./AttachmentLightbox"

interface MessageAttachmentsProps {
  message: Message
}

/**
 * Thumbnail row for a human message's linked image attachments, rendered
 * inside `MessageBubble`. Attachments come from `useMessageAttachments`'s
 * TanStack Query cache: for a message sent this session,
 * `useChatRoom`'s send-with-attachments sequencing has already seeded that
 * exact query key with the just-linked attachments (see
 * `use-chat-room.ts`), so this renders instantly with no loading flash; for
 * an older message loaded from history, the hook's own network fetch runs
 * normally.
 *
 * Renders nothing for a client-only optimistic entry (its id has no
 * server-side attachments to look up yet -- the query is disabled for it)
 * or once the list resolves to zero attachments.
 */
export function MessageAttachments({ message }: MessageAttachmentsProps) {
  const isPersisted = !message.id.startsWith("optimistic-")
  const attachmentsQuery = useMessageAttachments(
    message.room_id,
    message.id,
    isPersisted,
  )
  const [lightboxUrl, setLightboxUrl] = useState<string | null>(null)

  const attachments = attachmentsQuery.data ?? []
  if (attachments.length === 0) return null

  return (
    <>
      <Flex gap={2} wrap="wrap" maxW="240px" mt={2}>
        {attachments.map((attachment) => (
          <Image
            key={attachment.id}
            src={attachment.view_url}
            alt="Message attachment"
            w="24"
            h="24"
            objectFit="cover"
            rounded="lg"
            cursor="pointer"
            onClick={() => setLightboxUrl(attachment.view_url)}
          />
        ))}
      </Flex>
      <AttachmentLightbox
        imageUrl={lightboxUrl}
        onClose={() => setLightboxUrl(null)}
      />
    </>
  )
}
