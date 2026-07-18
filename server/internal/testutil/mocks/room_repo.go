// Package mocks provides shared, configurable in-memory fakes for the
// server's domain repository and service interfaces. It replaces the
// hand-written mock types that used to be duplicated across usecase and
// handler test files with one canonical implementation per interface.
//
// Unlike server/internal/testutil/postgres, this package carries no build
// tag: it must compile and run under plain `go test ./...` because it is a
// direct dependency of ordinary (non-integration) unit tests.
package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// RoomRepo is an in-memory, map-backed fake implementing
// room.RoomRepository. The zero value (mocks.RoomRepo{}) is ready to use;
// all maps are initialized lazily on first write.
//
// RoomRepo is safe for concurrent use.
type RoomRepo struct {
	mu      sync.Mutex
	Rooms   map[string]*room.Room
	Members map[string]map[string]*room.RoomMember // roomID -> userID -> member
}

// ensureInit lazily initializes the backing maps. Callers must hold mu.
func (r *RoomRepo) ensureInit() {
	if r.Rooms == nil {
		r.Rooms = make(map[string]*room.Room)
	}
	if r.Members == nil {
		r.Members = make(map[string]map[string]*room.RoomMember)
	}
}

// cloneTimePtr returns a fresh *time.Time pointing at the same instant as
// t, or nil if t is nil. Used by cloneRoom so a stored/returned Room's
// AIContextCutoffAt can never be mutated through a pointer some other
// caller (or the repo's own internal state) still holds.
func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

// cloneStringPtr returns a fresh *string with the same value as s, or nil
// if s is nil. Used by cloneRoom for AIProvider/AIModel/ForkedFromRoomID.
func cloneStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

// cloneRoom returns a deep-enough copy of rm: a struct copy plus fresh
// pointers for every one of Room's four pointer fields (AIContextCutoffAt,
// AIProvider, AIModel, ForkedFromRoomID) when non-nil. A plain struct copy
// (`cp := *rm`) still leaves each of those fields pointing at the very same
// value as rm's, since copying a struct copies its pointer fields by value,
// not what they point to -- so a caller mutating e.g. *cp.AIContextCutoffAt
// would silently alias the stored room. cloneRoom is used for every Room
// value stored into or read out of r.Rooms so no caller can ever observe or
// corrupt the repo's internal state through a shared pointer. Mirrors
// mocks.AttachmentRepo's cloneAttachment.
func cloneRoom(rm *room.Room) *room.Room {
	cp := *rm
	cp.AIContextCutoffAt = cloneTimePtr(rm.AIContextCutoffAt)
	cp.AIProvider = cloneStringPtr(rm.AIProvider)
	cp.AIModel = cloneStringPtr(rm.AIModel)
	cp.ForkedFromRoomID = cloneStringPtr(rm.ForkedFromRoomID)
	return &cp
}

// cloneRoomMember returns a shallow copy of m. RoomMember carries no
// pointer fields, so a struct copy alone is enough to stop a caller
// mutating the repo's stored membership through a returned pointer (or vice
// versa) -- unlike cloneRoom, there is nothing further to deep-copy.
func cloneRoomMember(m *room.RoomMember) *room.RoomMember {
	cp := *m
	return &cp
}

// SeedMember pre-populates a room membership directly, without requiring a
// corresponding room to exist in Rooms. This lets tests that only care about
// membership checks (e.g. message usecase tests) set up fixtures without
// going through Create — including tests that specifically exercise the
// "member exists but the room itself does not" case (RoomRepo.GetByID
// returning domain.ErrNotFound after a successful membership lookup). role
// is a plain string (e.g. "reader", "guest", "member", "admin", "master")
// converted to room.Role internally, so existing call sites written before
// Role became a typed enum keep working unchanged. Tests that additionally
// need a working RoomRepo.GetByID lookup (e.g. usecases that load the room
// to read AIContextCutoffAt) should also call SeedRoom.
func (r *RoomRepo) SeedMember(roomID, userID, role string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	if r.Members[roomID] == nil {
		r.Members[roomID] = make(map[string]*room.RoomMember)
	}
	r.Members[roomID][userID] = &room.RoomMember{RoomID: roomID, UserID: userID, Role: room.Role(role)}
}

// SeedRoom pre-populates a bare room (id only, no owner/name/description)
// with the given AIContextCutoffAt directly in Rooms, without requiring a
// corresponding membership or going through Create. This lets tests
// exercising code paths that call RoomRepo.GetByID (e.g.
// MessageUsecase.SendAIMessage/RegenerateAIMessage reading
// Room.AIContextCutoffAt) set up a minimal fixture. Pass a nil cutoff for
// "no cutoff configured".
func (r *RoomRepo) SeedRoom(roomID string, cutoff *time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()
	r.Rooms[roomID] = &room.Room{ID: roomID, AIContextCutoffAt: cloneTimePtr(cutoff)}
}

// Create persists a new room and automatically adds its owner as a member
// with role.RoleMaster, mirroring the production postgres.RoomRepository
// behavior. Stores a clone of rm (see cloneRoom) so later in-place mutation
// of the caller's own struct can't alias the repo's state.
func (r *RoomRepo) Create(_ context.Context, rm *room.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	r.Rooms[rm.ID] = cloneRoom(rm)
	if r.Members[rm.ID] == nil {
		r.Members[rm.ID] = make(map[string]*room.RoomMember)
	}
	r.Members[rm.ID][rm.OwnerID] = &room.RoomMember{
		ID: "seed-owner-membership", RoomID: rm.ID, UserID: rm.OwnerID, Role: room.RoleMaster,
	}
	return nil
}

// GetByID retrieves a room by ID. Returns domain.ErrNotFound if not present.
func (r *RoomRepo) GetByID(_ context.Context, id string) (*room.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneRoom(rm), nil
}

// ListByUserID returns all rooms the given user is a member of. A roomID
// with a membership but no corresponding Rooms entry (e.g. a test fixture
// built with SeedMember but no SeedRoom/Create) is skipped rather than
// cloned, since cloneRoom cannot deep-copy a nil *room.Room.
func (r *RoomRepo) ListByUserID(_ context.Context, userID string) ([]*room.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var rooms []*room.Room
	for roomID, members := range r.Members {
		if _, ok := members[userID]; !ok {
			continue
		}
		if rm, ok := r.Rooms[roomID]; ok {
			rooms = append(rooms, cloneRoom(rm))
		}
	}
	return rooms, nil
}

// ListByUserIDWithRole returns all rooms the given user is a member of,
// paired with the user's role in each room.
func (r *RoomRepo) ListByUserIDWithRole(_ context.Context, userID string) ([]*room.RoomWithRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*room.RoomWithRole
	for roomID, members := range r.Members {
		member, ok := members[userID]
		if !ok {
			continue
		}
		rm, ok := r.Rooms[roomID]
		if !ok {
			continue
		}
		result = append(result, &room.RoomWithRole{Room: cloneRoom(rm), Role: member.Role})
	}
	return result, nil
}

// UpdateDetails updates a room's Name and Description, mirroring
// postgres.RoomRepository.UpdateDetails's narrow-column-set contract.
// Returns domain.ErrNotFound if the room does not exist.
func (r *RoomRepo) UpdateDetails(_ context.Context, roomID, name, description string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.Name = name
	rm.Description = description
	rm.UpdatedAt = time.Now()
	return nil
}

// UpdateAIContextCutoff updates a room's AIContextCutoffAt, mirroring
// postgres.RoomRepository.UpdateAIContextCutoff's narrow-column-set
// contract. Returns domain.ErrNotFound if the room does not exist. Stores a
// clone of cutoff (see cloneTimePtr) so later mutation of the caller's own
// *time.Time can't alias the repo's state.
func (r *RoomRepo) UpdateAIContextCutoff(_ context.Context, roomID string, cutoff *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.AIContextCutoffAt = cloneTimePtr(cutoff)
	rm.UpdatedAt = time.Now()
	return nil
}

// UpdateAISettings updates a room's AIProvider and AIModel, mirroring
// postgres.RoomRepository.UpdateAISettings's narrow-column-set contract.
// Returns domain.ErrNotFound if the room does not exist. Stores clones of
// aiProvider/aiModel (see cloneStringPtr) so later mutation of the caller's
// own *string can't alias the repo's state.
func (r *RoomRepo) UpdateAISettings(_ context.Context, roomID string, aiProvider, aiModel *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.AIProvider = cloneStringPtr(aiProvider)
	rm.AIModel = cloneStringPtr(aiModel)
	rm.UpdatedAt = time.Now()
	return nil
}

// Delete removes a room by ID. Returns domain.ErrNotFound if the room does
// not exist.
func (r *RoomRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Rooms[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.Rooms, id)
	delete(r.Members, id)
	return nil
}

// AddMember adds a user to a room with the role specified on member. Stores
// a clone of member (see cloneRoomMember) so later in-place mutation of the
// caller's own struct can't alias the repo's state.
func (r *RoomRepo) AddMember(_ context.Context, member *room.RoomMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if r.Members[member.RoomID] == nil {
		r.Members[member.RoomID] = make(map[string]*room.RoomMember)
	}
	r.Members[member.RoomID][member.UserID] = cloneRoomMember(member)
	return nil
}

// GetMember retrieves a specific membership. Returns domain.ErrNotFound if
// not present.
func (r *RoomRepo) GetMember(_ context.Context, roomID, userID string) (*room.RoomMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	members, ok := r.Members[roomID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	member, ok := members[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneRoomMember(member), nil
}

// ListMembers returns all members of a room.
func (r *RoomRepo) ListMembers(_ context.Context, roomID string) ([]*room.RoomMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*room.RoomMember
	for _, member := range r.Members[roomID] {
		result = append(result, cloneRoomMember(member))
	}
	return result, nil
}

// RemoveMember removes a user from a room. Returns domain.ErrNotFound if
// the room has no members recorded or the membership does not exist.
func (r *RoomRepo) RemoveMember(_ context.Context, roomID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	members, ok := r.Members[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	if _, ok := members[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(members, userID)
	return nil
}

// UpdateMemberRole updates a single membership's role. Returns
// domain.ErrNotFound if the room or the membership does not exist. It also
// rechecks, while still holding r.mu (mirroring
// postgres.RoomRepository.UpdateMemberRole's `SELECT ... FOR UPDATE` lock
// via the same mutex TransferOwnership below also holds for its entire
// operation), whether userID is the room's current owner, returning
// room.ErrOwnerRoleProtected if so — this is what makes the fake correctly
// reject a role change that races a concurrent TransferOwnership, instead
// of silently demoting a room's brand-new owner out of RoleMaster.
func (r *RoomRepo) UpdateMemberRole(_ context.Context, roomID, userID string, role room.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	if rm.OwnerID == userID {
		return room.ErrOwnerRoleProtected
	}

	members, ok := r.Members[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	member, ok := members[userID]
	if !ok {
		return domain.ErrNotFound
	}
	member.Role = role
	return nil
}

// SetArchived flips the IsArchived flag on the given room. Returns
// domain.ErrNotFound if the room does not exist.
func (r *RoomRepo) SetArchived(_ context.Context, roomID string, archived bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.IsArchived = archived
	return nil
}

// TransferOwnership updates the fake Rooms map's OwnerID and both affected
// memberships' roles (new owner -> master, old owner -> admin), mirroring
// postgres.RoomRepository.TransferOwnership. Returns domain.ErrNotFound if
// the room or either membership does not exist.
func (r *RoomRepo) TransferOwnership(_ context.Context, roomID, oldOwnerID, newOwnerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	// Compare-and-swap on the current owner, mirroring
	// postgres.RoomRepository.TransferOwnership's `WHERE id = ... AND
	// owner_id = ...` guard: a stale oldOwnerID (a concurrent transfer
	// already moved ownership away from it) must be rejected rather than
	// silently overwritten.
	if rm.OwnerID != oldOwnerID {
		return domain.ErrNotFound
	}
	members, ok := r.Members[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	newOwnerMember, ok := members[newOwnerID]
	if !ok {
		return domain.ErrNotFound
	}
	oldOwnerMember, ok := members[oldOwnerID]
	if !ok {
		return domain.ErrNotFound
	}

	rm.OwnerID = newOwnerID
	newOwnerMember.Role = room.RoleMaster
	oldOwnerMember.Role = room.RoleAdmin
	return nil
}
