// Package room defines the room and membership entities and their repository port.
package room

import (
	"context"
	"time"
)

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

	// UpdateDetails updates only a room's name, description, and updated_at
	// fields, leaving ai_context_cutoff_at/ai_provider/ai_model (and every
	// other column) untouched. Split out from a single full-row Update
	// (which UpdateRoom, UpdateAIContextCutoff, and UpdateSettings used to
	// share) specifically so those usecases can no longer lost-update each
	// other: previously, each loaded the whole Room, mutated only the
	// field-group it owns, and wrote back every column, so a concurrent
	// sibling call landing between one call's read and write had its
	// change silently clobbered by the other's stale copy of the column it
	// never intended to touch. Returns ErrNotFound if the room does not
	// exist.
	UpdateDetails(ctx context.Context, roomID, name, description string, updatedAt time.Time) error

	// UpdateAIContextCutoff updates only a room's ai_context_cutoff_at and
	// updated_at fields, leaving name/description/ai_provider/ai_model
	// untouched. See UpdateDetails's GoDoc for why this is split out from a
	// full-row update. Returns ErrNotFound if the room does not exist.
	UpdateAIContextCutoff(ctx context.Context, roomID string, cutoff *time.Time, updatedAt time.Time) error

	// UpdateAISettings updates only a room's ai_provider, ai_model, and
	// updated_at fields, leaving name/description/ai_context_cutoff_at
	// untouched. See UpdateDetails's GoDoc for why this is split out from a
	// full-row update.
	//
	// aiProvider and aiModel each independently carry their own
	// nil/empty-string-sentinel/value convention straight through to the
	// implementation, rather than the caller resolving it against a
	// previously-read snapshot first:
	//   - nil leaves the corresponding column untouched at the database
	//     level -- implementations must not assign it any new value, so a
	//     concurrent call that only touches the other field cannot be
	//     lost-updated by this one.
	//   - a pointer to "" (empty string) clears the column to SQL NULL.
	//   - a pointer to any other non-empty value sets the column to that
	//     value.
	//
	// A caller that updates only one of the two fields must therefore pass
	// nil for the other rather than a value read from a prior GetByID, or
	// it reintroduces the same lost-update race this convention exists to
	// avoid. Implementations must apply both fields' assignments (or lack
	// thereof) and the updated_at write within a single atomic statement.
	// Returns ErrNotFound if the room does not exist.
	UpdateAISettings(ctx context.Context, roomID string, aiProvider, aiModel *string, updatedAt time.Time) error

	// Delete removes a room by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error

	// AddMember adds a user to a room with the given role.
	AddMember(ctx context.Context, member *RoomMember) error

	// GetMember retrieves a specific membership, with RoomMember.Username
	// populated via a JOIN against the users table (like ListMembers).
	// Returns ErrNotFound if not found.
	GetMember(ctx context.Context, roomID, userID string) (*RoomMember, error)

	// ListMembers returns all members of a room.
	ListMembers(ctx context.Context, roomID string) ([]*RoomMember, error)

	// RemoveMember removes a user from a room. It returns domain.ErrNotFound
	// if the membership (roomID, userID) does not exist, and
	// ErrOwnerRoleProtected if userID is the room's current owner (the
	// owner must transfer ownership via TransferOwnership before they can
	// be removed, never a plain removal).
	//
	// The owner recheck happens inside the same database transaction as the
	// room_members DELETE, under the same row lock (`SELECT ... FOR UPDATE`)
	// on rooms that UpdateMemberRole and TransferOwnership take — see
	// UpdateMemberRole's GoDoc for why this serializes a concurrent
	// TransferOwnership against this call instead of letting a stale
	// usecase-layer pre-check race it.
	RemoveMember(ctx context.Context, roomID, userID string) error

	// UpdateMemberRole updates a single membership's role. It returns
	// domain.ErrNotFound if the membership (roomID, userID) does not exist,
	// and ErrOwnerRoleProtected if userID is the room's current owner (the
	// owner's role may only change via TransferOwnership, never a plain
	// role edit).
	//
	// The owner recheck happens inside the same database transaction as the
	// role UPDATE, under a row lock (`SELECT ... FOR UPDATE`) on rooms taken
	// against the same rooms row that TransferOwnership's owner_id CAS
	// update locks — so a TransferOwnership call racing this one is
	// serialized against it rather than interleaved: whichever of the two
	// transactions acquires the row lock first runs to completion (commit or
	// rollback) before the other proceeds, and the loser's owner check
	// (here) or CAS (in TransferOwnership) then observes the winner's
	// already-committed state. Without that shared lock, a usecase-layer
	// pre-check (GetByID then "is targetUserID == OwnerID") could pass
	// before a concurrent TransferOwnership call makes targetUserID the new
	// owner, and then this method's UPDATE would still apply, leaving the
	// room with a master whose room_members.role was just downgraded by a
	// role change that should have been rejected.
	//
	// It does not itself enforce any RBAC invariant beyond the owner check
	// above — everything else (who may call this, which roles are valid
	// targets) is a usecase-layer concern; this method is otherwise a plain
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
	// two masters. The owner_id update is a compare-and-swap against
	// oldOwnerID (guarding against a concurrent transfer of the same room),
	// so it returns domain.ErrNotFound if the room does not exist, if
	// oldOwnerID is no longer the room's current owner (a concurrent
	// transfer already won), or if either membership row (oldOwnerID,
	// newOwnerID) does not exist -- rolling back any partial writes in every
	// case.
	TransferOwnership(ctx context.Context, roomID, oldOwnerID, newOwnerID string) error
}
