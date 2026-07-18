"use client"

import { CloseButton, Dialog, Image, Portal } from "@chakra-ui/react"

interface AttachmentLightboxProps {
  /** The full-resolution image URL to show, or `null` to render closed. */
  imageUrl: string | null
  onClose: () => void
}

/**
 * Full-resolution image viewer opened by clicking an attachment thumbnail in
 * `MessageAttachments`. Built on Chakra's `Dialog` compound component per
 * project convention (no bespoke modal, no new snippet) -- closable via
 * backdrop click, `Escape`, or the close button, all handled by `Dialog`
 * itself once `onOpenChange` is wired to `onClose`.
 */
export function AttachmentLightbox({ imageUrl, onClose }: AttachmentLightboxProps) {
  return (
    <Dialog.Root
      open={imageUrl !== null}
      onOpenChange={(e) => {
        if (!e.open) onClose()
      }}
      placement="center"
      size="full"
      lazyMount
      unmountOnExit
    >
      <Portal>
        <Dialog.Backdrop bg="blackAlpha.800" />
        <Dialog.Positioner>
          <Dialog.Content
            aria-label="Attachment preview"
            bg="transparent"
            boxShadow="none"
            display="flex"
            alignItems="center"
            justifyContent="center"
          >
            {imageUrl && (
              <Image
                src={imageUrl}
                alt="Full-size attachment"
                maxW="90vw"
                maxH="90vh"
                objectFit="contain"
                rounded="md"
              />
            )}
            <Dialog.CloseTrigger asChild>
              <CloseButton size="md" position="absolute" top="4" insetEnd="4" colorPalette="gray" bg="bg" />
            </Dialog.CloseTrigger>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  )
}
