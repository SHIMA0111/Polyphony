import { describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import { MemberListItem } from "./MemberListItem"
import type { Member, RoomRole } from "../types"

const { useRouterMock } = vi.hoisted(() => ({
  useRouterMock: vi.fn(() => ({ push: vi.fn() })),
}))

vi.mock("next/navigation", () => ({
  useRouter: useRouterMock,
  // `Provider` (via `src/test/render.tsx`) wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's SSR
  // styles; jsdom never streams, so a no-op is all component tests need
  // (see `(main)/layout.test.tsx`'s identical mock). `useLeaveRoom` (used by
  // this component's own row) also needs a mounted-app-router stand-in for
  // `useRouter`.
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Covers Step 37's Scope requirement: `MemberListItem`'s per-row
 * management controls (here, the `RolePicker` trigger) render only for a
 * viewer whose own role (`viewerRole`, sourced from `Room.role`) passes
 * `canManageMembers` (`admin`/`master`), and are absent for
 * `member`/`guest`/`reader` viewers — gating another member's row, not the
 * viewer's own (the fixture session user is `user-1`; this row is
 * `user-2`, so it is never the viewer's own row regardless of `viewerRole`).
 */
const otherMember: Member = {
  id: "member-2",
  room_id: "room-1",
  user_id: "user-2",
  username: "alice",
  role: "member",
  joined_at: "2026-01-02T00:00:00Z",
}

describe("MemberListItem", () => {
  it("shows a role-management control for an admin viewer", async () => {
    render(
      <MemberListItem roomId="room-1" member={otherMember} viewerRole="admin" />,
    )

    expect(await screen.findByRole("button", { name: /member/i })).toBeInTheDocument()
  })

  it.each<RoomRole>(["member", "guest", "reader"])(
    "hides role-management controls for a %s viewer",
    async (viewerRole) => {
      render(
        <MemberListItem roomId="room-1" member={otherMember} viewerRole={viewerRole} />,
      )

      await waitFor(() => expect(screen.getByText("alice")).toBeInTheDocument())
      expect(screen.queryByRole("button")).not.toBeInTheDocument()
    },
  )
})
