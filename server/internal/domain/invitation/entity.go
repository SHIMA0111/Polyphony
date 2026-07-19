// Package invitation defines the room invitation entity and its repository
// port. A room invitation lets a room Admin+ member bring another user into
// the room, either by targeting a known username (single-use) or by
// generating a reusable shareable link code, with an assigned Role and an
// expiry. It has zero dependencies beyond the standard library and
// server/internal/domain/room (for the Role type), so it can be imported
// from the usecase and interface layers without creating an upward
// dependency from the domain layer.
package invitation

import (
	"time"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// Status represents the lifecycle state of an Invitation.
type Status string

const (
	// StatusPending indicates the invitation has not yet been accepted,
	// rejected, or revoked. This is the initial state for every invitation.
	StatusPending Status = "pending"
	// StatusAccepted indicates a username-targeted invitation was accepted
	// by its invitee. Link invitations (InviteeID == nil) never transition
	// to StatusAccepted — they remain StatusPending and reusable until
	// expiry or explicit revocation.
	StatusAccepted Status = "accepted"
	// StatusRejected indicates a username-targeted invitation was rejected
	// by its invitee.
	StatusRejected Status = "rejected"
	// StatusRevoked indicates the invitation was explicitly revoked. No
	// code in this step sets this value (revocation is a follow-up step);
	// it exists so the column's domain is complete from the start.
	StatusRevoked Status = "revoked"
)

// Invitation represents an invitation for a user to join a room with a
// specific Role, created by an existing room member (the inviter).
//
// InviteeID distinguishes the two invitation kinds:
//   - Username-targeted: InviteeID is non-nil (resolved from the requested
//     username at creation time). Only that user may accept or reject it,
//     and it is single-use — accepting or rejecting moves it out of
//     StatusPending permanently.
//   - Link: InviteeID is nil. Any authenticated user holding InviteCode may
//     accept it, and it remains StatusPending (reusable) after being
//     accepted; RejectInvitation is not a valid operation on it.
type Invitation struct {
	ID        string
	RoomID    string
	InviterID string
	InviteeID *string
	// InviteCode is a unique, URL-safe code identifying this invitation,
	// usable to look it up via GetByCode regardless of invitation kind.
	InviteCode string
	// Role is the room.Role the invitee will be granted upon acceptance.
	Role      domainroom.Role
	Status    Status
	ExpiresAt time.Time
	CreatedAt time.Time
}
