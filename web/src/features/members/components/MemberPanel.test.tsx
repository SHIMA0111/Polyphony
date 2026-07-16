import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { fixtureRoom } from "@/features/rooms/api/handlers"
import { MemberPanel } from "./MemberPanel"
import type { Member, MemberListResponse } from "../types"

const { useRouterMock } = vi.hoisted(() => ({
  useRouterMock: vi.fn(() => ({ push: vi.fn() })),
}))

vi.mock("next/navigation", () => ({
  useRouter: useRouterMock,
  // See `MemberListItem.test.tsx`'s identical mock: `Provider` needs
  // `useServerInsertedHTML` (Emotion SSR flush) and each row's
  // `useLeaveRoom` needs a mounted-app-router stand-in for `useRouter`.
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Regression coverage for the wave-9 integration review's stale-member-list
 * finding (`e2e/members.spec.ts:114` timing out): `MemberPanel` is rendered
 * unconditionally by `MemberAvatarStack` so the drawer can animate, meaning
 * it never actually unmounts between opens. `useMembers`'s
 * `refetchOnMount: "always"` alone therefore only ever fires once per room
 * visit — `enabled: open` (see `use-members.ts`'s `UseMembersOptions`) is
 * what makes a *second* open issue a fresh request. This test drives the
 * `open` prop directly (closed -> open -> closed -> open) against a handler
 * that returns a different member list on each call, asserting the second
 * open actually observes the new data rather than the first call's cache.
 */
describe("MemberPanel - refetch on re-open", () => {
  it("refetches the member list every time the drawer transitions from closed to open", async () => {
    const memberSets: Member[][] = [
      [
        {
          id: "member-1",
          room_id: "room-1",
          user_id: "user-1",
          username: "alice",
          role: "master",
          joined_at: "2026-01-01T00:00:00Z",
        },
      ],
      [
        {
          id: "member-1",
          room_id: "room-1",
          user_id: "user-1",
          username: "alice",
          role: "master",
          joined_at: "2026-01-01T00:00:00Z",
        },
        {
          id: "member-2",
          room_id: "room-1",
          user_id: "user-2",
          username: "bob",
          role: "member",
          joined_at: "2026-01-02T00:00:00Z",
        },
      ],
    ]
    let callCount = 0
    server.use(
      http.get("/api/proxy/rooms/:roomId/members", () => {
        const members = memberSets[Math.min(callCount, memberSets.length - 1)]
        callCount += 1
        return HttpResponse.json<MemberListResponse>({ members })
      }),
    )

    const onOpenChange = vi.fn()
    const { rerender } = render(
      <MemberPanel room={fixtureRoom} open={false} onOpenChange={onOpenChange} />,
    )
    expect(callCount).toBe(0)

    // First open -> fetch #1 -> only "alice" is present yet.
    rerender(<MemberPanel room={fixtureRoom} open onOpenChange={onOpenChange} />)
    await waitFor(() => expect(screen.getByText("alice")).toBeInTheDocument())
    expect(screen.queryByText("bob")).not.toBeInTheDocument()

    // Close, then re-open -> fetch #2 -> "bob" (just joined) is now visible.
    rerender(<MemberPanel room={fixtureRoom} open={false} onOpenChange={onOpenChange} />)
    rerender(<MemberPanel room={fixtureRoom} open onOpenChange={onOpenChange} />)
    await waitFor(() => expect(screen.getByText("bob")).toBeInTheDocument())

    expect(callCount).toBe(2)
  })
})
