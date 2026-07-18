// Package group defines the personal group and group membership entities
// and their repository port. A group is a strictly personal (single-owner)
// address-book-style list of other users, maintained by one owner, that can
// later be used to batch-invite its entire membership into a room in a
// single call (see server/internal/usecase/group.GroupUsecase.BatchInviteToRoom).
// It has zero dependencies beyond the standard library, so it can be
// imported from the usecase and interface layers without creating an
// upward dependency from the domain layer.
package group

import "time"

// Group represents a personal, single-owner address-book-style list of
// other users, maintained by OwnerID. Groups do not carry any per-room role
// or permission of their own; a role is chosen once, per batch-invite call
// (see GroupUsecase.BatchInviteToRoom), not stored per group or per member.
type Group struct {
	ID          string
	OwnerID     string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GroupMember represents a single user's membership in a Group. Membership
// is a flat list — it carries no role or permission of its own.
type GroupMember struct {
	ID      string
	GroupID string
	UserID  string
	AddedAt time.Time
}

// GroupMemberWithUsername pairs a GroupMember with the member's Username,
// resolved via a JOIN against the users table. It is returned by
// GroupRepository.ListMembers so callers get human-readable display names
// without a separate user-directory lookup, mirroring
// domainroom.RoomMember.Username's JOIN-based population on
// RoomRepository.ListMembers.
type GroupMemberWithUsername struct {
	GroupMember
	Username string
}
