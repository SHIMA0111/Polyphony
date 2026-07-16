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

	// ErrStripeNotConfigured indicates a Stripe-backed billing endpoint
	// (checkout session creation, billing portal, subscription
	// cancellation) was called without STRIPE_SECRET_KEY configured. It is
	// mapped to HTTP 503, distinguishing "not set up yet" from a genuine
	// client error. GET /billing/plans never returns this error — it is a
	// pure config read that works even when Stripe is unconfigured.
	ErrStripeNotConfigured = errors.New("stripe is not configured")

	// ErrInvalidWebhookSignature indicates a Stripe webhook payload failed
	// signature verification (unknown/wrong STRIPE_WEBHOOK_SECRET, or a
	// tampered payload). POST /webhooks/stripe maps this to HTTP 400 — the
	// only case in which that endpoint returns a non-200 status.
	ErrInvalidWebhookSignature = errors.New("invalid stripe webhook signature")

	// ErrArchivedRoom indicates a new message (SendMessage/SendAIMessage)
	// was rejected because its target room is archived. A room is archived
	// from the moment a fork of it is created until the fork's background
	// copy job (usecase/room.RoomUsecase.runForkJob) reaches
	// roomfork.StatusCompleted (see domainroom.Room.IsArchived). Mapped to
	// HTTP 409 by handler.handleMessageError.
	ErrArchivedRoom = errors.New("room is archived")

	// ErrSubscriptionAlreadyExists indicates a subscriptions row already
	// exists for the given stripe_subscription_id (a unique-constraint
	// violation on SubscriptionRepository.Create). In normal operation the
	// webhook usecase avoids this by checking
	// SubscriptionRepository.GetByStripeSubscriptionID before deciding
	// whether to Create or Update; this sentinel only surfaces on a
	// concurrent-redelivery race.
	ErrSubscriptionAlreadyExists = errors.New("subscription already exists")

	// ErrStreamingUnsupported indicates the selected ai.LLMGateway
	// implementation does not support Stream at all (currently:
	// interface/gateway.GRPCClient, selected when
	// config.Config.LLMGatewayTransport is "grpc" -- see its Stream doc
	// comment). It is always wrapped together with ErrLLMGateway so existing
	// errors.Is(err, domain.ErrLLMGateway) call sites keep matching;
	// usecase/message.MessageUsecase.SendAIMessageStream additionally checks
	// errors.Is(err, ErrStreamingUnsupported) specifically to fall back to
	// the unary Complete path instead of failing the send outright.
	ErrStreamingUnsupported = errors.New("streaming not supported by this llm gateway transport")

	// ErrConflict indicates a request cannot be fulfilled because the
	// target resource is in a state incompatible with the requested
	// operation — e.g. MessageUsecase.RegenerateAIMessage rejecting a
	// regenerate call against an AI message that is still
	// domainmessage.MessageStatusStreaming (mid-stream, not yet finalized).
	// Mapped to HTTP 409 by handler.handleMessageError, alongside the more
	// specific ErrArchivedRoom which predates this generic sentinel.
	ErrConflict = errors.New("conflict")
)
