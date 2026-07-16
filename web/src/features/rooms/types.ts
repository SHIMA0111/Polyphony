/**
 * A chat room, exactly as returned by the Go API's `RoomResponse`
 * (`server/internal/interface/handler/dto.go`), reached client-side via the
 * Step 4 data-plane proxy at `/api/proxy/rooms*`.
 */
export interface Room {
  id: string
  name: string
  description: string
  owner_id: string
  created_at: string
  updated_at: string
}
