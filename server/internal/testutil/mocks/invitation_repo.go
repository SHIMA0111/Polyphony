package mocks

import (
	"context"
	"sort"
	"sync"
	"time"

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
	// CreateErr, if non-nil, is returned by every Create call instead of
	// persisting the invitation, for tests exercising a per-member
	// CreateInvitation failure that is neither invitation.ErrInviteCodeConflict
	// nor one of the two usecase-level skippable errors.
	CreateErr error

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

// cloneInvitation returns a deep-enough copy of inv: a struct copy plus a
// fresh *string for InviteeID when non-nil. A plain struct copy (`cp :=
// *inv`) still leaves cp.InviteeID pointing at the very same string as
// inv.InviteeID, since copying a struct copies its pointer fields by
// value, not what they point to -- so a caller mutating *cp.InviteeID would
// silently alias the stored invitation. cloneInvitation is used for every
// value stored into or read out of r.Invitations so no caller can ever
// observe or corrupt the repo's internal state through a shared InviteeID
// pointer. Mirrors mocks.AttachmentRepo's cloneAttachment.
func cloneInvitation(inv *invitation.Invitation) *invitation.Invitation {
	cp := *inv
	if inv.InviteeID != nil {
		inviteeID := *inv.InviteeID
		cp.InviteeID = &inviteeID
	}
	return &cp
}

// Create persists a new invitation. Returns invitation.ErrInviteCodeConflict
// if inv.InviteCode collides with an existing invitation's code, mirroring
// postgres.InvitationRepository's unique-constraint behavior. If CreateErr
// is non-nil, it is returned immediately instead and nothing is persisted.
func (r *InvitationRepo) Create(_ context.Context, inv *invitation.Invitation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	if r.CreateErr != nil {
		return r.CreateErr
	}

	for _, existing := range r.Invitations {
		if existing.InviteCode == inv.InviteCode {
			return invitation.ErrInviteCodeConflict
		}
	}
	r.Invitations[inv.ID] = cloneInvitation(inv)
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
	return cloneInvitation(inv), nil
}

// GetByCode retrieves an invitation by its unique invite code. Returns
// domain.ErrNotFound if not present.
func (r *InvitationRepo) GetByCode(_ context.Context, code string) (*invitation.Invitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, inv := range r.Invitations {
		if inv.InviteCode == code {
			return cloneInvitation(inv), nil
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
			return cloneInvitation(inv), nil
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
			result = append(result, cloneInvitation(inv))
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
			result = append(result, cloneInvitation(inv))
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
// for tests: it holds r.mu across the pending-status/expiry check -- a CAS
// via acceptTransitionCASLocked when transitionStatus is true, or a plain
// read-and-compare against StatusPending/expires_at when transitionStatus
// is false, mirroring the real transaction's `SELECT ... FOR UPDATE` -- and
// the AddMember callback, so a concurrent UpdateStatus call for the same
// invitation ID is serialized against this one exactly as a real DB
// transaction's row lock would serialize it. If AddMember fails after a
// successful status CAS, the status is rolled back to what it was
// immediately before the CAS, mirroring the real transaction's rollback on
// a failed room_members insert, rather than leaving the invitation stuck
// StatusAccepted with no corresponding member row. See the AddMember
// field's doc comment.
func (r *InvitationRepo) AcceptTx(ctx context.Context, invitationID string, expectedStatus invitation.Status, transitionStatus bool, member *domainroom.RoomMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if transitionStatus {
		var priorStatus invitation.Status
		if inv, ok := r.Invitations[invitationID]; ok {
			priorStatus = inv.Status
		}
		if err := r.acceptTransitionCASLocked(invitationID, expectedStatus); err != nil {
			return err
		}
		if r.AddMember == nil {
			return nil
		}
		if err := r.AddMember(ctx, member); err != nil {
			r.Invitations[invitationID].Status = priorStatus
			return err
		}
		return nil
	}

	// Reusable link invitations never run the CAS above, so check the
	// invitation's current status and expiry here instead -- mirroring
	// postgres.InvitationRepository.AcceptTx's `SELECT ... FOR UPDATE`
	// re-check -- rather than admitting the member regardless of status.
	inv, ok := r.Invitations[invitationID]
	if !ok {
		return domain.ErrNotFound
	}
	if inv.Status != invitation.StatusPending {
		return domain.ErrInvitationNotPending
	}
	if time.Now().After(inv.ExpiresAt) {
		return domain.ErrInvitationExpired
	}
	if r.AddMember == nil {
		return nil
	}
	return r.AddMember(ctx, member)
}

// acceptTransitionCASLocked is AcceptTx's transitionStatus==true CAS body,
// callable while r.mu is already held (see AcceptTx's doc comment). It
// mirrors postgres.acceptTransitionCAS: it transitions id from
// expectedStatus to StatusAccepted only if the invitation is not yet
// expired. Unlike updateStatusLocked (shared with the general-purpose
// UpdateStatus, used by e.g. RevokeInvitation), it is not reused there:
// revoking or rejecting an already-expired invitation must still succeed,
// so the expiry gate belongs only here.
func (r *InvitationRepo) acceptTransitionCASLocked(id string, expectedStatus invitation.Status) error {
	inv, ok := r.Invitations[id]
	if !ok {
		return domain.ErrNotFound
	}
	if inv.Status != expectedStatus {
		return domain.ErrInvitationNotPending
	}
	if time.Now().After(inv.ExpiresAt) {
		return domain.ErrInvitationExpired
	}
	inv.Status = invitation.StatusAccepted
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
