"use client"

import { Status } from "@chakra-ui/react"
import { Tooltip } from "@/components/ui/tooltip"
import type { ConnectionStatus as ConnectionStatusValue } from "../hooks/use-room-socket"

interface ConnectionStatusProps {
  status: ConnectionStatusValue
}

/** Tooltip label + dot color for each `useRoomSocket` status value. */
const STATUS_DISPLAY: Record<
  ConnectionStatusValue,
  { label: string; colorPalette: "green" | "yellow" | "gray" }
> = {
  connected: { label: "Connected", colorPalette: "green" },
  connecting: { label: "Connecting…", colorPalette: "yellow" },
  reconnecting: { label: "Reconnecting…", colorPalette: "yellow" },
  offline: { label: "Offline", colorPalette: "gray" },
}

/**
 * Small colored-dot indicator reflecting `useRoomSocket`'s live connection
 * status, rendered in `ChatRoomHeader`. Built from `@chakra-ui/react`'s
 * `Status` compound component (a dot, not the `Badge` pill) wrapped in the
 * existing `Tooltip` snippet (`web/src/components/ui/tooltip.tsx`) for the
 * human-readable label, per `.claude/rules/chakra-ui.md`'s import
 * priority — no new snippet is added.
 *
 * Renders its `"offline"` (gray, inert) state whenever the app is in MSW
 * mock mode, since `useRoomSocket` never attempts a real connection there.
 */
export function ConnectionStatus({ status }: ConnectionStatusProps) {
  const { label, colorPalette } = STATUS_DISPLAY[status]

  return (
    <Tooltip content={label}>
      <Status.Root
        aria-label={`Connection status: ${label}`}
        colorPalette={colorPalette}
      >
        <Status.Indicator />
      </Status.Root>
    </Tooltip>
  )
}
