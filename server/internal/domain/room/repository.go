// Package room defines the room and membership entities and their repository port.
package room

import (
	"context"
	"errors"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
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

	// UpdateDetails updates a room's Name and Description (and UpdatedAt). It
	// is a narrow, dedicated setter — like SetArchived below — that touches
	// only these two columns plus updated_at, not the whole row. This
	// matters because UpdateDetails, UpdateAIContextCutoff, and
	// UpdateAISettings are each driven by a separate usecase endpoint
	// (RoomUsecase.UpdateRoom / UpdateAIContextCutoff / UpdateSettings) that
	// can be called concurrently for the same room: a single full-row
	// Update(ctx, *Room) — the shape this interface used before this
	// three-way split — would have each caller load a Room snapshot, mutate
	// only the field(s) its own endpoint owns, and write the whole struct
	// back, silently reverting any column a concurrent sibling call had just
	// changed in between (a classic lost update). Splitting Update into
	// three single-purpose setters, one per disjoint column group, makes
	// that race structurally impossible: each setter only ever touches its
	// own columns. Returns ErrNotFound if the room does not exist.
	UpdateDetails(ctx context.Context, roomID, name, description string) error

	// UpdateAIContextCutoff sets or clears a room's AIContextCutoffAt column
	// (and UpdatedAt). A nil cutoff clears the restriction. See
	// UpdateDetails's GoDoc for why this is a narrow setter rather than
	// routing through a full-row update. Returns ErrNotFound if the room
	// does not exist.
	UpdateAIContextCutoff(ctx context.Context, roomID string, cutoff *time.Time) error

	// UpdateAISettings conditionally sets a room's AIProvider and AIModel
	// columns (and UpdatedAt) in a single atomic UPDATE: setProvider and
	// setModel are independent field-presence flags, and each field's
	// corresponding *string value is only consulted (and only written) when
	// its flag is true. A false flag leaves that column completely
	// untouched at the database level — the implementation must express
	// this as a conditional column assignment (e.g. SQL CASE WHEN) evaluated
	// within one UPDATE statement, not as a read-modify-write, so that two
	// concurrent calls each setting a disjoint field (e.g. one setting only
	// AIProvider, the other only AIModel) can never lose one call's write
	// to the other's stale snapshot. When a flag is true, a nil value
	// clears that column to SQL NULL and a non-nil value sets it to
	// *value — this is the final value to persist, not a "leave unchanged"
	// sentinel. The usecase layer (RoomUsecase.UpdateSettings) is
	// responsible for translating its own nil/empty-string-sentinel/value
	// request convention into this set-flag/value pair per field before
	// calling this method. See UpdateDetails's GoDoc for why this touches
	// only these two columns rather than routing through a full-row
	// update. Returns ErrNotFound if the room does not exist.
	UpdateAISettings(ctx context.Context, roomID string, setProvider bool, aiProvider *string, setModel bool, aiModel *string) error

	// Delete removes a room by ID. Returns ErrNotFound if not found.
	Delete(ctx context.Context, id string) error

	// AddMember adds a user to a room with the given role.
	AddMember(ctx context.Context, member *RoomMember) error

	// GetMember retrieves a specific membership. Returns ErrNotFound if not found.
	GetMember(ctx context.Context, roomID, userID string) (*RoomMember, error)

	// ListMembers returns all members of a room.
	ListMembers(ctx context.Context, roomID string) ([]*RoomMember, error)

	// RemoveMember removes a user from a room. It returns
	// domain.ErrNotFound if the membership (roomID, userID) does not exist.
	// It does not itself enforce caller-side RBAC (who is allowed to call
	// it) — that remains a usecase-layer concern. It does, however, lock the
	// room's row and recheck, under that lock, whether userID is the room's
	// current owner, returning ErrOwnerRoleProtected if so — mirroring
	// UpdateMemberRole's same lock-and-recheck guarantee (see its GoDoc for
	// the race this closes against a concurrent TransferOwnership).
	// Implementations must serialize against TransferOwnership on the same
	// room row for this guarantee to hold.
	RemoveMember(ctx context.Context, roomID, userID string) error

	// UpdateMemberRole updates a single membership's role. It returns
	// domain.ErrNotFound if the membership (roomID, userID) does not exist.
	// It does not itself enforce caller-side RBAC (who is allowed to call
	// it) — that remains a usecase-layer concern. It does, however, lock the
	// room's row and recheck, under that lock, whether userID is the room's
	// current owner, returning ErrOwnerRoleProtected if so: this recheck
	// exists specifically to close a race against a concurrent
	// TransferOwnership call for the same room (the usecase layer's own
	// owner check, read via a separate non-transactional GetByID before
	// calling this method, can otherwise be stale by the time this method's
	// write lands — see the postgres implementation's GoDoc for the two
	// directions that race can go wrong). Implementations must serialize
	// against TransferOwnership on the same room row for this guarantee to
	// hold.
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
	// two masters. The rooms.owner_id update is a compare-and-swap against
	// oldOwnerID (not a blind write), guarding against two concurrent
	// transfers for the same room racing on a stale oldOwnerID. It returns
	// domain.ErrNotFound if the room does not exist, if oldOwnerID is no
	// longer the current owner (a concurrent transfer already moved
	// ownership, i.e. a stale-owner CAS conflict), or if either membership
	// row (oldOwnerID, newOwnerID) does not exist, rolling back any partial
	// writes.
	TransferOwnership(ctx context.Context, roomID, oldOwnerID, newOwnerID string) error
}

// GetMemberOrForbidden loads the caller's membership in roomID via repo,
// translating a missing membership (domain.ErrNotFound) into
// domain.ErrForbidden so that a non-member can never distinguish "room does
// not exist" from "room exists but I'm not a member of it" via the returned
// error.
//
// Shared by usecase/message.MessageUsecase, usecase/room.RoomUsecase, and
// usecase/invitation.InvitationUsecase (L6 post-review dedup finding): each
// used to define its own identical getMember method wrapping exactly this
// translation. They now each keep a thin same-named getMember method that
// just forwards here, preserving each usecase's existing call sites and
// doc-comment cross-references.
func GetMemberOrForbidden(ctx context.Context, repo RoomRepository, roomID, userID string) (*RoomMember, error) {
	member, err := repo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrForbidden
		}
		return nil, err
	}
	return member, nil
}
