import type { RoomRole } from "@/features/members/types"

/**
 * Moved here from `features/members/lib/roles.ts` (M5 post-review dedup):
 * `features/messages/components/ChatRoom.tsx` and
 * `features/rooms/components/RoomSettingsDrawer.tsx` both imported it
 * directly from the `members` feature, violating the no-cross-feature-
 * imports rule. `RoomRole` itself still comes from `@/features/members/types`
 * (matching the existing precedent of `features/rooms/types.ts`'s `Room.role`
 * field, which does the same type-only cross-feature import) -- only this
 * role-comparison logic moved, not the type definition.
 */

/**
 * The 5-tier role order, lowest-to-highest privilege — a client-side mirror
 * of `server/internal/domain/room/role.go`'s `roleRank` map. Used only to
 * compare two roles' relative privilege; the numeric rank itself is not
 * exposed since callers should compare via {@link roleAtLeast} instead of
 * relying on the index.
 */
export const ROLE_ORDER = [
  "reader",
  "guest",
  "member",
  "admin",
  "master",
] as const

/**
 * Whether `role` carries at least as much privilege as `min`, per
 * {@link ROLE_ORDER}'s rank.
 */
export function roleAtLeast(role: RoomRole, min: RoomRole): boolean {
  return ROLE_ORDER.indexOf(role) >= ROLE_ORDER.indexOf(min)
}

/**
 * Whether `role` can send messages in a room — mirrors the server's
 * `Role.Allows(ActionSendMessage)` (`guest` and above).
 *
 * UI-gating only: the server independently rejects a `reader`'s send
 * attempt regardless of what this function returns.
 */
export function canSendMessage(role: RoomRole): boolean {
  return roleAtLeast(role, "guest")
}

/**
 * Whether `role` can invoke AI in a room — mirrors the server's
 * `Role.Allows(ActionInvokeAI)` (`member` and above).
 *
 * UI-gating only: the server independently rejects a `guest`'s AI-invoke
 * attempt regardless of what this function returns.
 */
export function canInvokeAI(role: RoomRole): boolean {
  return roleAtLeast(role, "member")
}

/**
 * Whether `role` can manage members (invite, change role, transfer
 * ownership entry point) — mirrors the server's
 * `Role.Allows(ActionManageMembers)` (`admin` and above).
 *
 * UI-gating only: the server independently rejects a non-admin's
 * member-management request regardless of what this function returns.
 */
export function canManageMembers(role: RoomRole): boolean {
  return roleAtLeast(role, "admin")
}

/** Whether `role` is the room's current owner (exactly `master`). */
export function isOwnerRole(role: RoomRole): boolean {
  return role === "master"
}
