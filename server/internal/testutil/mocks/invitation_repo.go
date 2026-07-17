package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
)

// InvitationRepo is an in-memory, map-backed fake implementing
// invitation.InvitationRepository. The zero value (mocks.InvitationRepo{})
// is ready to use; the backing map is initialized lazily on first write.
//
// InvitationRepo is safe for concurrent use.
type InvitationRepo struct {
	mu          sync.Mutex
	Invitations map[string]*invitation.Invitation // keyed by ID
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

// UpdateStatus updates the status of the invitation identified by id.
// Returns domain.ErrNotFound if the invitation does not exist.
func (r *InvitationRepo) UpdateStatus(_ context.Context, id string, status invitation.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, ok := r.Invitations[id]
	if !ok {
		return domain.ErrNotFound
	}
	inv.Status = status
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
