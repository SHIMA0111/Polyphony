package group

import "context"

// GroupRepository defines persistence operations for personal groups and
// their memberships. Implementations translate "no such row" conditions
// into domain.ErrNotFound (see server/internal/domain/errors.go); they do
// not themselves enforce any ownership/RBAC rule — those are usecase-layer
// concerns (see server/internal/usecase/group.GroupUsecase), mirroring the
// division of responsibility documented on
// server/internal/domain/room/repository.go.
type GroupRepository interface {
	// Create persists a new group.
	Create(ctx context.Context, g *Group) error

	// GetByID retrieves a group by ID. Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*Group, error)

	// ListByOwnerID returns all groups owned by the given user.
	ListByOwnerID(ctx context.Context, ownerID string) ([]*Group, error)

	// Update updates a group's mutable fields (name, description,
	// updated_at). Returns domain.ErrNotFound if the group does not exist.
	Update(ctx context.Context, g *Group) error

	// Delete removes a group by ID, cascading to its group_members rows
	// via ON DELETE CASCADE. Returns domain.ErrNotFound if the group does
	// not exist.
	Delete(ctx context.Context, id string) error

	// AddMember adds a user to a group.
	AddMember(ctx context.Context, member *GroupMember) error

	// GetMember retrieves a specific group membership by group ID and user
	// ID. Returns domain.ErrNotFound if the membership does not exist.
	GetMember(ctx context.Context, groupID, userID string) (*GroupMember, error)

	// ListMembers returns all members of a group, with each member's
	// Username resolved via a JOIN against the users table.
	ListMembers(ctx context.Context, groupID string) ([]*GroupMemberWithUsername, error)

	// RemoveMember removes a user from a group. Returns domain.ErrNotFound
	// if the membership does not exist.
	RemoveMember(ctx context.Context, groupID, userID string) error
}
