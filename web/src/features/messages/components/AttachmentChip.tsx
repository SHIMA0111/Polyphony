"use client"

import { Box, IconButton, Image, ProgressCircle } from "@chakra-ui/react"
import { RefreshCw, X } from "lucide-react"
import { Tooltip } from "@/components/ui/tooltip"
import type { StagedAttachment } from "@/features/messages/hooks/use-attachment-staging"

interface AttachmentChipProps {
  attachment: StagedAttachment
  onRemove: (id: string) => void
  onRetry: (id: string) => void
}

/**
 * One staged-attachment preview chip, rendered in the horizontally-scrolling
 * row `MessageInput` shows above its textarea: a thumbnail (from the
 * client-only `previewUrl` object URL), a progress ring overlay while the
 * upload is in flight, a retry affordance if it failed, and an
 * always-available remove button.
 */
export function AttachmentChip({ attachment, onRemove, onRetry }: AttachmentChipProps) {
  const { id, previewUrl, status, progress, errorMessage } = attachment

  return (
    <Box position="relative" flexShrink={0} w="16" h="16">
      <Box
        w="full"
        h="full"
        rounded="lg"
        overflow="hidden"
        borderWidth="1px"
        borderColor={status === "error" ? "red.300" : "border"}
        opacity={status === "uploading" ? 0.5 : 1}
      >
        <Image src={previewUrl} alt="" w="full" h="full" objectFit="cover" />
      </Box>

      {status === "uploading" && (
        <Box
          position="absolute"
          inset={0}
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <ProgressCircle.Root value={progress} size="sm">
            <ProgressCircle.Circle>
              <ProgressCircle.Track />
              <ProgressCircle.Range />
            </ProgressCircle.Circle>
          </ProgressCircle.Root>
        </Box>
      )}

      {status === "error" && (
        <Tooltip content={errorMessage ?? "Upload failed"}>
          <IconButton
            aria-label="Retry upload"
            size="2xs"
            colorPalette="red"
            variant="solid"
            position="absolute"
            bottom="-2"
            left="-2"
            rounded="full"
            onClick={() => onRetry(id)}
          >
            <RefreshCw size={10} />
          </IconButton>
        </Tooltip>
      )}

      <IconButton
        aria-label="Remove attachment"
        size="2xs"
        variant="solid"
        colorPalette="gray"
        position="absolute"
        top="-2"
        right="-2"
        rounded="full"
        onClick={() => onRemove(id)}
      >
        <X size={10} />
      </IconButton>
    </Box>
  )
}
