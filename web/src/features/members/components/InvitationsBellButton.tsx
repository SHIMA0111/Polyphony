"use client"

import { useState } from "react"
import { Badge, Box, Button, Popover, Portal } from "@chakra-ui/react"
import { Mail } from "lucide-react"
import { useMyInvitations } from "../hooks/use-my-invitations"
import { InvitationsInbox } from "./InvitationsInbox"

/**
 * Top-bar entry point (Step 16's persistent `(main)/layout.tsx`) for the
 * caller's own pending invitations: an icon button with a numeric badge
 * when any exist, opening `InvitationsInbox` in a `Popover.Root` (mirroring
 * `ModelSelector.tsx`'s positioning pattern).
 */
export function InvitationsBellButton() {
  const [open, setOpen] = useState(false)
  const invitationsQuery = useMyInvitations()
  const pendingCount = invitationsQuery.data?.invitations.length ?? 0

  return (
    <Popover.Root
      open={open}
      onOpenChange={(e) => setOpen(e.open)}
      positioning={{ placement: "bottom-end" }}
      lazyMount
      unmountOnExit
    >
      <Box position="relative">
        <Popover.Trigger asChild>
          <Button
            aria-label="Invitations"
            variant="ghost"
            rounded="full"
            p={0}
            h={9}
            w={9}
          >
            <Mail size={18} />
          </Button>
        </Popover.Trigger>
        {pendingCount > 0 && (
          <Badge
            position="absolute"
            top={-1}
            right={-1}
            size="xs"
            variant="solid"
            colorPalette="red"
            rounded="full"
            minW={4}
            h={4}
            px={1}
            fontSize="2xs"
            display="flex"
            alignItems="center"
            justifyContent="center"
          >
            {pendingCount}
          </Badge>
        )}
      </Box>
      <Portal>
        <Popover.Positioner>
          <Popover.Content w="sm" p={2}>
            <Popover.Body p={0}>
              <InvitationsInbox />
            </Popover.Body>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
