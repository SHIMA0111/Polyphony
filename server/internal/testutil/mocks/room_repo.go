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
	mu sync.Mutex
	// Rooms is the backing store of rooms, keyed by room ID; access only
	// while holding mu.
	Rooms map[string]*room.Room
	// Members is the backing store of room memberships, keyed by roomID
	// then userID; access only while holding mu.
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
	r.Rooms[roomID] = &room.Room{ID: roomID, AIContextCutoffAt: cutoff}
}

// Create persists a new room and automatically adds its owner as a member
// with role.RoleMaster, mirroring the production postgres.RoomRepository
// behavior.
func (r *RoomRepo) Create(_ context.Context, rm *room.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	r.Rooms[rm.ID] = rm
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
	return rm, nil
}

// ListByUserID returns all rooms the given user is a member of.
func (r *RoomRepo) ListByUserID(_ context.Context, userID string) ([]*room.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var rooms []*room.Room
	for roomID, members := range r.Members {
		rm, ok := r.Rooms[roomID]
		if !ok {
			continue // orphan membership with no corresponding room
		}
		if _, ok := members[userID]; ok {
			rooms = append(rooms, rm)
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
		result = append(result, &room.RoomWithRole{Room: rm, Role: member.Role})
	}
	return result, nil
}

// Update updates room fields. Returns domain.ErrNotFound if the room does
// not exist.
func (r *RoomRepo) Update(_ context.Context, rm *room.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Rooms[rm.ID]; !ok {
		return domain.ErrNotFound
	}
	r.Rooms[rm.ID] = rm
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

// AddMember adds a user to a room with the role specified on member.
func (r *RoomRepo) AddMember(_ context.Context, member *room.RoomMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if r.Members[member.RoomID] == nil {
		r.Members[member.RoomID] = make(map[string]*room.RoomMember)
	}
	r.Members[member.RoomID][member.UserID] = member
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
	return member, nil
}

// ListMembers returns all members of a room.
func (r *RoomRepo) ListMembers(_ context.Context, roomID string) ([]*room.RoomMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*room.RoomMember
	for _, member := range r.Members[roomID] {
		result = append(result, member)
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
// domain.ErrNotFound if the membership does not exist.
func (r *RoomRepo) UpdateMemberRole(_ context.Context, roomID, userID string, role room.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
