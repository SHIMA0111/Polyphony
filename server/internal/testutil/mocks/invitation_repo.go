package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// InvitationRepo is an in-memory, map-backed fake implementing
// invitation.InvitationRepository. The zero value (mocks.InvitationRepo{})
// is ready to use; the backing map is initialized lazily on first write.
//
// InvitationRepo is safe for concurrent use.
type InvitationRepo struct {
	mu          sync.Mutex
	Invitations map[string]*invitation.Invitation // keyed by ID

	// AddMember, when set, is invoked by AcceptTx (after a successful status
	// CAS, or immediately for a link invitation) to actually add the room
	// member. Tests wire this to the same *mocks.RoomRepo instance the
	// usecase under test uses, so InvitationRepo.AcceptTx and
	// mocks.RoomRepo.AddMember together approximate the Postgres
	// implementation's single-transaction atomicity: AcceptTx holds r.mu for
	// its entire duration (status CAS + this callback), so a concurrent
	// UpdateStatus call (e.g. from RejectInvitation) blocks until AcceptTx
	// finishes and then re-evaluates its own CAS against the now-updated
	// status -- the same serialization a real DB row lock would provide. If
	// nil, AcceptTx performs only the status transition, matching prior
	// behavior for tests that don't care about membership.
	AddMember func(ctx context.Context, member *domainroom.RoomMember) error
}

func (r *InvitationRepo) ensureInit() {
	if r.Invitations == nil {
		r.Invitations = make(map[string]*invitation.Invitation)
	}
}

// Create persists a new invitation. Returns invitation.ErrInviteCodeConflict
// if inv.InviteCode collides with an existing invitation's code, mirroring
// postgres.InvitationRepository's unique-constraint behavior.
func (r *InvitationRepo) Create(_ context.Context, inv *invitation.Invitation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	for _, existing := range r.Invitations {
		if existing.InviteCode == inv.InviteCode {
			return invitation.ErrInviteCodeConflict
		}
	}
	cp := *inv
	r.Invitations[inv.ID] = &cp
	return nil
}

// GetByID retrieves an invitation by its ID. Returns domain.ErrNotFound if
// not present.
func (r *InvitationRepo) GetByID(_ context.Context, id string) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, ok := r.Invitations[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *inv
	return &cp, nil
}

// GetByCode retrieves an invitation by its unique invite code. Returns
// domain.ErrNotFound if not present.
func (r *InvitationRepo) GetByCode(_ context.Context, code string) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, inv := range r.Invitations {
		if inv.InviteCode == code {
			cp := *inv
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

// GetPendingByRoomAndInvitee retrieves the pending, username-targeted
// invitation for the given (roomID, inviteeID) pair. Returns
// domain.ErrNotFound if no such invitation exists.
func (r *InvitationRepo) GetPendingByRoomAndInvitee(_ context.Context, roomID, inviteeID string) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, inv := range r.Invitations {
		if inv.RoomID == roomID && inv.InviteeID != nil && *inv.InviteeID == inviteeID &&
			inv.Status == invitation.StatusPending {
			cp := *inv
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

// ListByRoomID returns all invitations created for the given room, ordered
// by creation time descending.
func (r *InvitationRepo) ListByRoomID(_ context.Context, roomID string) ([]*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*invitation.Invitation
	for _, inv := range r.Invitations {
		if inv.RoomID == roomID {
			cp := *inv
			result = append(result, &cp)
		}
	}
	sortInvitationsByCreatedAtDesc(result)
	return result, nil
}

// ListPendingByInviteeID returns all pending invitations targeted at the
// given invitee, ordered by creation time descending.
func (r *InvitationRepo) ListPendingByInviteeID(_ context.Context, inviteeID string) ([]*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*invitation.Invitation
	for _, inv := range r.Invitations {
		if inv.InviteeID != nil && *inv.InviteeID == inviteeID && inv.Status == invitation.StatusPending {
			cp := *inv
			result = append(result, &cp)
		}
	}
	sortInvitationsByCreatedAtDesc(result)
	return result, nil
}

// UpdateStatus performs a compare-and-swap status transition, mirroring
// postgres.InvitationRepository's UpdateStatus: it only updates the
// invitation's status if its current status still equals expectedStatus.
// The whole check-then-set is done while holding r.mu, so this is atomic
// with respect to other InvitationRepo calls -- the same guarantee the real
// postgres UPDATE ... WHERE ... provides at the row level.
//
// Returns domain.ErrNotFound if the invitation does not exist, or
// domain.ErrInvitationNotPending if it exists but its current status does
// not equal expectedStatus.
func (r *InvitationRepo) UpdateStatus(_ context.Context, id string, newStatus, expectedStatus invitation.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateStatusLocked(id, newStatus, expectedStatus)
}

// updateStatusLocked is UpdateStatus's body, callable by AcceptTx while it
// already holds r.mu (see AcceptTx's doc comment on why it holds the lock
// across both the status CAS and the AddMember callback).
func (r *InvitationRepo) updateStatusLocked(id string, newStatus, expectedStatus invitation.Status) error {
	inv, ok := r.Invitations[id]
	if !ok {
		return domain.ErrNotFound
	}
	if inv.Status != expectedStatus {
		return domain.ErrInvitationNotPending
	}
	inv.Status = newStatus
	return nil
}

// AcceptTx approximates postgres.InvitationRepository.AcceptTx's atomicity
// for tests: it holds r.mu across both the status CAS (when
// transitionStatus is true) and the AddMember callback, so a concurrent
// UpdateStatus call for the same invitation ID is serialized against this
// one exactly as a real DB transaction's row lock would serialize it. See
// the AddMember field's doc comment.
func (r *InvitationRepo) AcceptTx(ctx context.Context, invitationID string, expectedStatus invitation.Status, transitionStatus bool, member *domainroom.RoomMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if transitionStatus {
		if err := r.updateStatusLocked(invitationID, invitation.StatusAccepted, expectedStatus); err != nil {
			return err
		}
	}

	if r.AddMember != nil {
		return r.AddMember(ctx, member)
	}
	return nil
}

// sortInvitationsByCreatedAtDesc sorts invitations in place by CreatedAt
// descending, matching the ORDER BY clause used by
// postgres.InvitationRepository's list queries.
func sortInvitationsByCreatedAtDesc(invs []*invitation.Invitation) {
	sort.Slice(invs, func(i, j int) bool {
		return invs[i].CreatedAt.After(invs[j].CreatedAt)
	})
}
