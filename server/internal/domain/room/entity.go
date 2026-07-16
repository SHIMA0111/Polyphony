// Package room defines the room and membership entities and their repository port.
package room

import "time"

// Room represents a chat room where users and AI interact.
type Room struct {
	ID          string
	Name        string
	Description string
	OwnerID     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RoomMember represents a user's membership in a room, including their
// authorization Role within it.
type RoomMember struct {
	ID       string
	RoomID   string
	UserID   string
	Role     Role
	JoinedAt time.Time
}

// RoomWithRole pairs a Room with a specific user's Role in that room. It is
// returned by RoomRepository.ListByUserIDWithRole (and used by the usecase
// layer as the return type of GetRoom/CreateRoom/UpdateRoom/ListRooms) so
// callers can surface the requesting user's permission level without a
// second repository round-trip.
type RoomWithRole struct {
	Room *Room
	Role Role
}
