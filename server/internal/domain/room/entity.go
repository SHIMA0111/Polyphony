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

// RoomMember represents a user's membership in a room.
type RoomMember struct {
	ID       string
	RoomID   string
	UserID   string
	Role     string
	JoinedAt time.Time
}
