// Package invitation defines the room invitation entity and its repository port.
package invitation

import (
	"context"
	"errors"
)

// ErrInviteCodeConflict indicates Create failed because the generated
// InviteCode collided with an existing invitation's code (a unique-
// constraint violation). The usecase layer treats this as retryable:
// generate a new code and call Create again once.
var ErrInviteCodeConflict = errors.New("invite code conflict")

// InvitationRepository defines persistence operations for room invitations.
type InvitationRepository interface {
	// Create persists a new Invitation. Callers are responsible for
	// generating a unique ID and InviteCode before calling Create. Returns
	// ErrInviteCodeConflict if inv.InviteCode collides with an existing
	// invitation's code (a unique-constraint violation); the caller may
	// retry with a freshly generated code.
	Create(ctx context.Context, inv *Invitation) error

	// GetByID retrieves an invitation by its ID. Returns domain.ErrNotFound
	// if no invitation with that ID exists.
	GetByID(ctx context.Context, id string) (*Invitation, error)

	// GetByCode retrieves an invitation by its unique InviteCode. Returns
	// domain.ErrNotFound if no invitation with that code exists.
	GetByCode(ctx context.Context, code string) (*Invitation, error)

	// GetPendingByRoomAndInvitee retrieves the pending, username-targeted
	// invitation for the given (roomID, inviteeID) pair, if one exists.
	// Returns domain.ErrNotFound if no such invitation exists (including
	// when one exists but is not StatusPending). Used to reject duplicate
	// pending invitations for the same user in the same room.
	GetPendingByRoomAndInvitee(ctx context.Context, roomID, inviteeID string) (*Invitation, error)

	// ListByRoomID returns all invitations created for the given room,
	// ordered by creation time descending, regardless of status.
	ListByRoomID(ctx context.Context, roomID string) ([]*Invitation, error)

	// ListPendingByInviteeID returns all StatusPending invitations whose
	// InviteeID equals inviteeID, ordered by creation time descending. Link
	// invitations (InviteeID == nil) are never returned, since they are not
	// targeted at a specific user.
	ListPendingByInviteeID(ctx context.Context, inviteeID string) ([]*Invitation, error)

	// UpdateStatus updates the status of the invitation identified by id.
	// Returns domain.ErrNotFound if no invitation with that ID exists.
	UpdateStatus(ctx context.Context, id string, status Status) error
}
