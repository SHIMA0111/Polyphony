import { InviteAcceptView } from "@/features/members/components/InviteAcceptView"

/**
 * Server Component wrapper for the invite-link landing page, following this
 * repo's existing thin-page/feature-component split (e.g. `RoomsPage` ->
 * `RoomList`, `ChatRoomPage` -> `ChatRoom`): unwraps the route's `code`
 * param and hands it to the `"use client"` `InviteAcceptView`, which owns
 * all data-fetching and mutation logic for previewing and
 * accepting/rejecting the invitation.
 *
 * Guarded behind the same auth requirement as every other `(main)` route
 * (see `web/src/middleware.ts`'s `/invite/:path*` matcher entry) — an
 * unauthenticated visitor is redirected to `/login` before this ever
 * renders.
 */
export default async function InvitePage({
  params,
}: {
  params: Promise<{ code: string }>
}) {
  const { code } = await params
  return <InviteAcceptView code={code} />
}
