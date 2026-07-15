// Package domain defines shared domain-level sentinel errors used across entities and use cases.
package domain

import "errors"

var (
	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrEmailAlreadyExists indicates a user with the given email already exists.
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrUsernameAlreadyExists indicates a user with the given username already exists.
	ErrUsernameAlreadyExists = errors.New("username already exists")

	// ErrKratosIdentityAlreadyLinked indicates the given Ory Kratos identity
	// ID is already linked to a different local user (a unique constraint
	// violation on users.kratos_identity_id).
	ErrKratosIdentityAlreadyLinked = errors.New("kratos identity already linked to another user")

	// ErrInvalidCredentials indicates the provided credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidToken indicates the provided token is invalid or expired.
	ErrInvalidToken = errors.New("invalid token")

	// ErrUnauthorized indicates the request lacks valid authentication.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the authenticated user lacks permission.
	ErrForbidden = errors.New("forbidden")

	// ErrLLMGateway indicates an error communicating with the LLM Gateway.
	ErrLLMGateway = errors.New("llm gateway error")

	// ErrInvalidMessageType indicates the message type is not valid for the operation.
	ErrInvalidMessageType = errors.New("invalid message type")

	// ErrAttachmentAlreadyLinked indicates an attachment has already been
	// linked to a message and cannot be attached again.
	ErrAttachmentAlreadyLinked = errors.New("attachment already linked to a message")

	// ErrUnsupportedMimeType indicates an attachment upload was requested
	// with a MIME type outside the supported allow-list.
	ErrUnsupportedMimeType = errors.New("unsupported mime type")

	// ErrAttachmentTooLarge indicates an attachment upload was requested
	// with a declared size exceeding the maximum allowed.
	ErrAttachmentTooLarge = errors.New("attachment too large")

	// ErrInvitationExpired indicates an invitation's ExpiresAt has already
	// passed at the time it was checked (e.g. on accept).
	ErrInvitationExpired = errors.New("invitation expired")

	// ErrInvitationNotPending indicates an operation that requires an
	// invitation to be in StatusPending (e.g. accept, reject) was attempted
	// on an invitation that has already been accepted, rejected, or revoked.
	ErrInvitationNotPending = errors.New("invitation is not pending")

	// ErrAlreadyMember indicates the target user is already a member of the
	// room, so accepting the invitation (or otherwise adding them) would
	// create a duplicate room_members row.
	ErrAlreadyMember = errors.New("user is already a member of the room")

	// ErrInvitationAlreadyExists indicates a pending, username-targeted
	// invitation already exists for the same (room, invitee) pair.
	ErrInvitationAlreadyExists = errors.New("invitation already exists")

	// ErrInsufficientBalance indicates the room owner's token balance is at
	// or below zero, so an AI invocation was rejected before calling the LLM
	// Gateway. See usecase/billing.BillingUsecase.CheckBalance.
	ErrInsufficientBalance = errors.New("insufficient token balance")
)

// IsLLMGatewayError checks if the error wraps ErrLLMGateway.
func IsLLMGatewayError(err error) bool {
	return errors.Is(err, ErrLLMGateway)
}
