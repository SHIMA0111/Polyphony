// Package invitation defines the room invitation entity and its repository port.
package invitation

import (
	"context"
	"errors"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
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

	// UpdateStatus transitions the invitation identified by id to status, but
	// only if its current status is still expectedStatus (compare-and-swap,
	// via a `WHERE id = ... AND status = ...` update). This closes a
	// lost-update race between two concurrent callers transitioning the same
	// invitation (e.g. a concurrent Accept and Reject both reading
	// StatusPending before either writes): only the first writer's UPDATE
	// matches the WHERE clause, so the second's affects zero rows and must
	// be rejected rather than blindly overwriting the first writer's status.
	//
	// Returns domain.ErrNotFound if no invitation with that ID exists at
	// all, and domain.ErrInvitationNotPending if it exists but its current
	// status is not expectedStatus.
	UpdateStatus(ctx context.Context, id string, status, expectedStatus Status) error

	// AcceptTx atomically performs an invitation accept, verifying the
	// invitation's pending, non-expired status atomically inside a single
	// database transaction in both modes before ever inserting into
	// room_members:
	//   - transitionStatus == true (a username-targeted invitation): it
	//     first transitions the invitation identified by invitationID from
	//     expectedStatus to StatusAccepted exactly as UpdateStatus's CAS
	//     does, additionally gated on the row not being expired, and only
	//     proceeds if that transition succeeds.
	//   - transitionStatus == false (a reusable link invitation, which
	//     never changes status): it instead locks the invitation row and
	//     confirms its current status is still StatusPending and it has not
	//     expired, rejecting the accept otherwise.
	// Either way, member is only inserted into room_members once that check
	// passes, with both the check and the insert committing or rolling back
	// together in a single database transaction.
	//
	// This closes two races UpdateStatus's CAS alone cannot: without a
	// shared transaction, two callers could each pass their own pre-check
	// (GetByID + a stale status read) before either writes, and separately
	// call AddMember and UpdateStatus, leaving a room_members row inserted
	// for an invitation whose status a concurrent Reject just set to
	// StatusRejected (or vice versa: a status flip to StatusAccepted with no
	// corresponding member row, if AddMember's separate call failed after
	// UpdateStatus succeeded). For link invitations specifically — which
	// never run the CAS above — it also closes a TOCTOU window where a
	// revoke or expiry landing after the usecase's own pre-check read, but
	// before this call, would otherwise still admit the member. It also
	// closes a third race not covered by a usecase-level expiry pre-check
	// alone: an invitation expiring in the gap between that pre-check and
	// this call would otherwise still be accepted, in either mode.
	//
	// Returns domain.ErrNotFound if the invitation does not exist,
	// domain.ErrInvitationNotPending if the invitation's current status is
	// not StatusPending — checked against expectedStatus via CAS when
	// transitionStatus is true, or read-and-compared to StatusPending under
	// row lock when transitionStatus is false — domain.ErrInvitationExpired
	// if the status check passed but expires_at is in the past, and
	// domain.ErrAlreadyMember if member.UserID is already a member of
	// member.RoomID (a unique-constraint violation on room_members, e.g. a
	// concurrent accept of a different invitation into the same room won
	// first).
	AcceptTx(ctx context.Context, invitationID string, expectedStatus Status, transitionStatus bool, member *domainroom.RoomMember) error
}
