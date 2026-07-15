// Package room defines the room and membership entities and their repository port.
package room

import "context"

// RoomRepository defines persistence operations for rooms and memberships.
type RoomRepository interface {
	// Create persists a new room and initializes its sequence counter.
	// The room owner is automatically added as a member.
	Create(ctx context.Context, room *Room) error

	// GetByID retrieves a room by ID. Returns ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*Room, error)

	// ListByUserID returns all rooms the given user is a member of.
	ListByUserID(ctx context.Context, userID string) ([]*Room, error)

	// ListByUserIDWithRole returns all rooms the given user is a member of,
	// together with the user's Role in each room, as a single query (no
	// N+1 GetMember lookups). Use this instead of ListByUserID whenever the
	// caller's per-room role is needed (e.g. to populate RoomResponse.Role).
	ListByUserIDWithRole(ctx context.Context, userID string) ([]*RoomWithRole, error)

	// Update updates room fields. Returns ErrNotFound if not found.
	Update(ctx context.Context, room *Room) error

	// Delete removes a room by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error

	// AddMember adds a user to a room with the given role.
	AddMember(ctx context.Context, member *RoomMember) error

	// GetMember retrieves a specific membership. Returns ErrNotFound if not found.
	GetMember(ctx context.Context, roomID, userID string) (*RoomMember, error)

	// ListMembers returns all members of a room.
	ListMembers(ctx context.Context, roomID string) ([]*RoomMember, error)

	// RemoveMember removes a user from a room.
	RemoveMember(ctx context.Context, roomID, userID string) error
}
