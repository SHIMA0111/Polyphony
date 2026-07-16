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

	// Update updates room fields, including AIProvider and AIModel (persisted
	// as part of a normal update alongside name/description/
	// ai_context_cutoff_at — there is no separate settings-only persistence
	// method). Returns ErrNotFound if not found.
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

	// UpdateMemberRole updates a single membership's role. It returns
	// domain.ErrNotFound if the membership (roomID, userID) does not exist.
	// It does not itself enforce any RBAC or "owner role is protected"
	// invariant — those are usecase-layer concerns; this method is a plain
	// persistence operation.
	UpdateMemberRole(ctx context.Context, roomID, userID string, role Role) error

	// SetArchived flips a room's is_archived flag. It is a narrow, dedicated
	// setter (rather than routing through Update) so the room-fork worker
	// (usecase/room.RoomUsecase.runForkJob) can flip archival status
	// without racing a concurrent room-settings edit (Update) that loads,
	// mutates, and writes back a whole Room struct. Returns
	// domain.ErrNotFound if the room does not exist.
	SetArchived(ctx context.Context, roomID string, archived bool) error

	// TransferOwnership atomically updates rooms.owner_id to newOwnerID,
	// sets the new owner's room_members.role to RoleMaster, and sets the
	// previous owner's (oldOwnerID) room_members.role to RoleAdmin, all
	// within a single transaction so a room is never observed with zero or
	// two masters. It returns domain.ErrNotFound if either the room or
	// either membership row (oldOwnerID, newOwnerID) does not exist,
	// rolling back any partial writes.
	TransferOwnership(ctx context.Context, roomID, oldOwnerID, newOwnerID string) error
}
