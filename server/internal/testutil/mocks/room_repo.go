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

	// BeforeGetByID, when set, is invoked synchronously at the very start of
	// every GetByID call, before it acquires mu or touches the backing
	// store. Concurrency tests use this hook to build a rendezvous barrier
	// (e.g. a sync.WaitGroup that every call Done()s then Wait()s on) so
	// every participating goroutine's "snapshot" GetByID call provably
	// completes before any of them proceeds to its subsequent write --
	// exercising the intended concurrent-write race window deterministically
	// instead of leaving the interleaving up to goroutine scheduling luck.
	BeforeGetByID func()
}

// cloneTimePtr returns a pointer to a copy of the time.Time t points to, or
// nil if t is nil.
//
// A plain struct-literal copy of room.Room (`cloned := *rm`) only copies the
// AIContextCutoffAt pointer value, not the time.Time it points to -- so the
// clone and the internally-stored room would keep sharing the same
// *time.Time. GetByID/ListByUserID/ListByUserIDWithRole use cloneTimePtr on
// their way out, and UpdateAIContextCutoff uses it on its way in, so no
// caller on either side can mutate a time.Time reachable from the mock's
// stored state through a pointer it merely received or handed out --
// matching the independence a real Postgres round-trip provides.
func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
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
//
// Returns a clone, never the internally-stored pointer: usecases routinely
// load a room via GetByID and then locally mutate fields on the returned
// struct before persisting a change (e.g. RoomUsecase.UpdateRoom sets
// rm.Name/rm.Description on its own copy). Handing out the live pointer
// would let concurrent callers race on those same struct fields with no
// locking at all -- a real data race, not just a logic bug -- since
// mutations wouldn't go through r.mu the way UpdateDetails/
// UpdateAIContextCutoff/etc. do.
func (r *RoomRepo) GetByID(_ context.Context, id string) (*room.Room, error) {
	if r.BeforeGetByID != nil {
		r.BeforeGetByID()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cloned := *rm
	cloned.AIContextCutoffAt = cloneTimePtr(rm.AIContextCutoffAt)
	return &cloned, nil
}

// ListByUserID returns all rooms the given user is a member of. Each
// returned *room.Room is a clone (see GetByID's GoDoc for why).
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
			cloned := *rm
			cloned.AIContextCutoffAt = cloneTimePtr(rm.AIContextCutoffAt)
			rooms = append(rooms, &cloned)
		}
	}
	return rooms, nil
}

// ListByUserIDWithRole returns all rooms the given user is a member of,
// paired with the user's role in each room. Each returned *room.Room is a
// clone (see GetByID's GoDoc for why).
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
		cloned := *rm
		cloned.AIContextCutoffAt = cloneTimePtr(rm.AIContextCutoffAt)
		result = append(result, &room.RoomWithRole{Room: &cloned, Role: member.Role})
	}
	return result, nil
}

// UpdateDetails updates only a room's Name, Description, and UpdatedAt
// fields, mirroring postgres.RoomRepository.UpdateDetails's partial-update
// shape (AIContextCutoffAt is left untouched). Returns domain.ErrNotFound if
// the room does not exist.
func (r *RoomRepo) UpdateDetails(_ context.Context, roomID, name, description string, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.Name = name
	rm.Description = description
	rm.UpdatedAt = updatedAt
	return nil
}

// UpdateAIContextCutoff updates only a room's AIContextCutoffAt and
// UpdatedAt fields, mirroring postgres.RoomRepository.UpdateAIContextCutoff's
// partial-update shape (Name/Description are left untouched). Returns
// domain.ErrNotFound if the room does not exist.
func (r *RoomRepo) UpdateAIContextCutoff(_ context.Context, roomID string, cutoff *time.Time, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	rm.AIContextCutoffAt = cloneTimePtr(cutoff)
	rm.UpdatedAt = updatedAt
	return nil
}

// UpdateAISettings updates only a room's AIProvider, AIModel, and UpdatedAt
// fields, mirroring postgres.RoomRepository.UpdateAISettings's
// partial-update shape (Name/Description/AIContextCutoffAt are left
// untouched) and its nil/empty-string-sentinel/value convention: a nil
// field leaves the corresponding stored field untouched, a pointer to ""
// clears it to nil, and any other pointer value sets it to a copy of the
// pointed-to value. The whole read-then-conditionally-write happens while
// holding r.mu, giving the same atomicity guarantee the real UPDATE ...
// CASE WHEN statement provides against a concurrent call touching only the
// other field. Returns domain.ErrNotFound if the room does not exist.
func (r *RoomRepo) UpdateAISettings(_ context.Context, roomID string, aiProvider, aiModel *string, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	if aiProvider != nil {
		if *aiProvider == "" {
			rm.AIProvider = nil
		} else {
			v := *aiProvider
			rm.AIProvider = &v
		}
	}
	if aiModel != nil {
		if *aiModel == "" {
			rm.AIModel = nil
		} else {
			v := *aiModel
			rm.AIModel = &v
		}
	}
	rm.UpdatedAt = updatedAt
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
//
// Returns a clone, never the internally-stored pointer (see GetByID's
// GoDoc for why): ChangeMemberRole, for instance, mutates
// target.Role on its own copy of the value GetMember returns.
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
	cloned := *member
	return &cloned, nil
}

// ListMembers returns all members of a room. Each returned *room.RoomMember
// is a clone (see GetByID's GoDoc for why).
func (r *RoomRepo) ListMembers(_ context.Context, roomID string) ([]*room.RoomMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*room.RoomMember
	for _, member := range r.Members[roomID] {
		cloned := *member
		result = append(result, &cloned)
	}
	return result, nil
}

// RemoveMember removes a membership, mirroring
// postgres.RoomRepository.RemoveMember's owner-protection contract: it
// returns room.ErrOwnerRoleProtected if userID is roomID's current owner,
// checked (and, together with the rest of this method, executed) while
// holding r.mu -- the same atomicity a real DB transaction's row lock on
// rooms provides against a concurrent TransferOwnership call, see
// TransferOwnership's GoDoc. Returns domain.ErrNotFound if the room or the
// membership does not exist.
func (r *RoomRepo) RemoveMember(_ context.Context, roomID, userID string) error {
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
	if _, ok := members[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(members, userID)
	return nil
}

// UpdateMemberRole updates a single membership's role, mirroring
// postgres.RoomRepository.UpdateMemberRole's owner-protection contract: it
// returns room.ErrOwnerRoleProtected if userID is roomID's current owner,
// checked (and, together with the rest of this method, executed) while
// holding r.mu -- the same atomicity a real DB transaction's row lock on
// rooms provides against a concurrent TransferOwnership call, see
// TransferOwnership's GoDoc. Returns domain.ErrNotFound if the room or the
// membership does not exist.
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

// TransferOwnership updates the fake Rooms map's OwnerID and both affected
// memberships' roles (new owner -> master, old owner -> admin), mirroring
// postgres.RoomRepository.TransferOwnership. Returns domain.ErrNotFound if
// the room or either membership does not exist.
// TransferOwnership mirrors postgres.RoomRepository.TransferOwnership,
// including its compare-and-swap on the room's current owner: it returns
// domain.ErrNotFound (without mutating anything) if rm.OwnerID no longer
// equals oldOwnerID, e.g. because a concurrent TransferOwnership call
// already won. The whole check-then-set runs while holding r.mu, giving the
// same atomicity guarantee the real `UPDATE ... WHERE owner_id = ...`
// provides at the row level.
func (r *RoomRepo) TransferOwnership(_ context.Context, roomID, oldOwnerID, newOwnerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rm, ok := r.Rooms[roomID]
	if !ok {
		return domain.ErrNotFound
	}
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
