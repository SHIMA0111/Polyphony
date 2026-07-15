import { Badge, type BadgeProps } from "@chakra-ui/react"
import type { RoomRole } from "../types"

/** Display label + `colorPalette` for each of the 5 room roles. */
const ROLE_META: Record<RoomRole, { label: string; colorPalette: string }> = {
  reader: { label: "Reader", colorPalette: "gray" },
  guest: { label: "Guest", colorPalette: "cyan" },
  member: { label: "Member", colorPalette: "blue" },
  admin: { label: "Admin", colorPalette: "purple" },
  master: { label: "Owner", colorPalette: "orange" },
}

interface RoleBadgeProps extends Omit<BadgeProps, "colorPalette" | "children"> {
  role: RoomRole
}

/**
 * A small colored `Badge` labeling a room role, reused everywhere a
 * member's or invitation's role is displayed (`MemberListItem`,
 * `RolePicker`, `InvitationsInbox`, `/invite/[code]`).
 */
export function RoleBadge({ role, ...badgeProps }: RoleBadgeProps) {
  const meta = ROLE_META[role]

  return (
    <Badge size="sm" variant="subtle" colorPalette={meta.colorPalette} {...badgeProps}>
      {meta.label}
    </Badge>
  )
}
