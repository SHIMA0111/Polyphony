package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
)

// GroupRepo is an in-memory, map-backed fake implementing
// group.GroupRepository. The zero value (mocks.GroupRepo{}) is ready to
// use; all backing maps are initialized lazily on first write.
//
// Group members are additionally tracked in Order, a per-group slice of
// user IDs in insertion order, so ListMembers returns a deterministic order
// (added-at ascending) instead of Go's randomized map iteration order —
// this matters for tests asserting exactly which member a batch operation
// processed first (see GroupUsecase.BatchInviteToRoom's short-circuit
// behavior).
//
// GroupRepo is safe for concurrent use.
type GroupRepo struct {
	mu      sync.Mutex
	Groups  map[string]*group.Group
	Members map[string]map[string]*group.GroupMember // groupID -> userID -> member
	Order   map[string][]string                      // groupID -> userIDs in insertion order
	// Usernames resolves a userID to its username, used to populate
	// GroupMemberWithUsername.Username on ListMembers, mirroring the
	// postgres.GroupRepository's JOIN against the users table. Tests must
	// populate this (directly, or via SeedMember) for every user they add
	// as a group member.
	Usernames map[string]string
}

func (r *GroupRepo) ensureInit() {
	if r.Groups == nil {
		r.Groups = make(map[string]*group.Group)
	}
	if r.Members == nil {
		r.Members = make(map[string]map[string]*group.GroupMember)
	}
	if r.Order == nil {
		r.Order = make(map[string][]string)
	}
	if r.Usernames == nil {
		r.Usernames = make(map[string]string)
	}
}

// SeedMember pre-populates a group membership directly (and records
// username for username resolution on ListMembers), without requiring a
// corresponding group to exist in Groups.
func (r *GroupRepo) SeedMember(groupID, userID, username string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	if r.Members[groupID] == nil {
		r.Members[groupID] = make(map[string]*group.GroupMember)
	}
	if _, exists := r.Members[groupID][userID]; !exists {
		r.Order[groupID] = append(r.Order[groupID], userID)
	}
	r.Members[groupID][userID] = &group.GroupMember{GroupID: groupID, UserID: userID}
	r.Usernames[userID] = username
}

// Create persists a new group.
func (r *GroupRepo) Create(_ context.Context, g *group.Group) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	r.Groups[g.ID] = g
	return nil
}

// GetByID retrieves a group by ID. Returns domain.ErrNotFound if not present.
func (r *GroupRepo) GetByID(_ context.Context, id string) (*group.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	g, ok := r.Groups[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return g, nil
}

// ListByOwnerID returns all groups owned by the given user.
func (r *GroupRepo) ListByOwnerID(_ context.Context, ownerID string) ([]*group.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*group.Group
	for _, g := range r.Groups {
		if g.OwnerID == ownerID {
			result = append(result, g)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// Update updates group fields. Returns domain.ErrNotFound if the group does
// not exist.
func (r *GroupRepo) Update(_ context.Context, g *group.Group) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Groups[g.ID]; !ok {
		return domain.ErrNotFound
	}
	r.Groups[g.ID] = g
	return nil
}

// Delete removes a group by ID, along with its members. Returns
// domain.ErrNotFound if the group does not exist.
func (r *GroupRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Groups[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.Groups, id)
	delete(r.Members, id)
	delete(r.Order, id)
	return nil
}

// AddMember adds a user to a group.
func (r *GroupRepo) AddMember(_ context.Context, member *group.GroupMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if r.Members[member.GroupID] == nil {
		r.Members[member.GroupID] = make(map[string]*group.GroupMember)
	}
	if _, exists := r.Members[member.GroupID][member.UserID]; !exists {
		r.Order[member.GroupID] = append(r.Order[member.GroupID], member.UserID)
	}
	r.Members[member.GroupID][member.UserID] = member
	return nil
}

// GetMember retrieves a specific membership. Returns domain.ErrNotFound if
// not present.
func (r *GroupRepo) GetMember(_ context.Context, groupID, userID string) (*group.GroupMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	members, ok := r.Members[groupID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	member, ok := members[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return member, nil
}

// ListMembers returns all members of a group, in insertion order, with
// usernames resolved via r.Usernames.
func (r *GroupRepo) ListMembers(_ context.Context, groupID string) ([]*group.GroupMemberWithUsername, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*group.GroupMemberWithUsername
	for _, userID := range r.Order[groupID] {
		member, ok := r.Members[groupID][userID]
		if !ok {
			continue
		}
		result = append(result, &group.GroupMemberWithUsername{
			GroupMember: *member,
			Username:    r.Usernames[userID],
		})
	}
	return result, nil
}

// RemoveMember removes a user from a group. Returns domain.ErrNotFound if
// the group has no members recorded or the membership does not exist.
func (r *GroupRepo) RemoveMember(_ context.Context, groupID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	members, ok := r.Members[groupID]
	if !ok {
		return domain.ErrNotFound
	}
	if _, ok := members[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(members, userID)
	for i, id := range r.Order[groupID] {
		if id == userID {
			r.Order[groupID] = append(r.Order[groupID][:i], r.Order[groupID][i+1:]...)
			break
		}
	}
	return nil
}
