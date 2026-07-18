package handler

import "time"

// --- Auth DTOs ---

// RegisterRequest is the request body for POST /auth/register. All fields
// are required.
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest is the request body for POST /auth/login. Both email and
// password are required.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResponse is the response body returned after successful authentication.
// It contains an access token and its type (e.g., "Bearer").
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// --- Room DTOs ---

// CreateRoomRequest is the request body for POST /rooms. Name is required;
// Description is optional.
type CreateRoomRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateRoomRequest is the request body for PUT /rooms/:roomId. Name is
// required; Description is optional.
type UpdateRoomRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateRoomAIContextCutoffRequest is the request body for
// PATCH /rooms/:roomId/ai-context-cutoff. A JSON null or omitted
// cutoff_at clears the room's AI context cutoff; a valid RFC3339 timestamp
// sets it.
type UpdateRoomAIContextCutoffRequest struct {
	CutoffAt *time.Time `json:"cutoff_at"`
}

// UpdateRoomSettingsRequest is the request body for
// PATCH /rooms/:roomId/settings. Both fields are optional and each
// independently follows RoomUsecase.UpdateSettings's nil/empty-string
// convention: a field omitted from the JSON body (or explicit JSON null,
// which decodes identically for a *string field) leaves the corresponding
// stored value unchanged; an explicit empty string ("") clears it back to
// NULL; any other value sets it.
type UpdateRoomSettingsRequest struct {
	AIProvider *string `json:"ai_provider"`
	AIModel    *string `json:"ai_model"`
}

// RoomResponse is the response body for a room. Role is the requesting
// user's role in this room (e.g. "reader", "guest", "member", "admin",
// "master"), serialized as the plain string value of domainroom.Role so
// clients can do direct string comparisons. AIContextCutoffAt is nil when
// the room has no AI context cutoff configured. AIProvider/AIModel are nil
// when the room has no per-room AI default configured (see
// UpdateRoomSettingsRequest / PATCH /rooms/:roomId/settings), in which case
// AI requests fall through to the deployment-wide default
// (Config.DefaultAIModel). ForkedFromRoomID is nil unless this room was
// created via POST /rooms/:roomId/fork, in which case it names the source
// room. IsArchived is true from the moment a fork of this room is created
// until its background copy job (see ForkJobResponse) completes; while
// true, POST .../messages and .../messages/ai on this room return HTTP 409.
type RoomResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	OwnerID           string     `json:"owner_id"`
	Role              string     `json:"role"`
	AIContextCutoffAt *time.Time `json:"ai_context_cutoff_at"`
	AIProvider        *string    `json:"ai_provider"`
	AIModel           *string    `json:"ai_model"`
	ForkedFromRoomID  *string    `json:"forked_from_room_id"`
	IsArchived        bool       `json:"is_archived"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// MemberResponse is the JSON response representation of a single room
// membership. Username is populated on the member-list endpoint (which
// JOINs against the users table) and left empty ("") on responses built
// from non-JOINed lookups such as the role-change endpoint.
type MemberResponse struct {
	ID       string    `json:"id"`
	RoomID   string    `json:"room_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// MemberListResponse is the response body for GET /rooms/:roomId/members.
type MemberListResponse struct {
	Members []MemberResponse `json:"members"`
}

// ChangeMemberRoleRequest is the request body for
// PATCH /rooms/:roomId/members/:userId/role. Role must be one of "reader",
// "guest", "member", or "admin" — granting "master" through this endpoint
// is rejected; use the ownership-transfer endpoint instead.
type ChangeMemberRoleRequest struct {
	Role string `json:"role"`
}

// TransferOwnershipRequest is the request body for
// PATCH /rooms/:roomId/owner. NewOwnerID is required and must already be a
// member of the room.
type TransferOwnershipRequest struct {
	NewOwnerID string `json:"new_owner_id"`
}

// ForkJobResponse is the JSON representation of a room fork job's progress
// (see roomusecase.RoomUsecase.ForkRoom/GetForkJobStatus). Status is one of
// "pending", "running", "completed", or "failed" (the plain string value of
// roomfork.Status). TotalMessages is 0 while Status == "pending".
// ErrorMessage is non-nil only when Status == "failed".
type ForkJobResponse struct {
	ID             string    `json:"id"`
	SourceRoomID   string    `json:"source_room_id"`
	NewRoomID      string    `json:"new_room_id"`
	Status         string    `json:"status"`
	TotalMessages  int64     `json:"total_messages"`
	CopiedMessages int64     `json:"copied_messages"`
	ErrorMessage   *string   `json:"error_message"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RoomForkResponse is the response body for POST /rooms/:roomId/fork: the
// newly created (archived) room paired with the room-fork Job tracking the
// background copy into it. Poll GET /rooms/:roomId/fork-jobs/:jobId with
// Job.ID until Status == "completed", at which point NewRoom.IsArchived (as
// last observed here) has flipped to false server-side.
type RoomForkResponse struct {
	Job     ForkJobResponse `json:"job"`
	NewRoom RoomResponse    `json:"new_room"`
}

// --- Message DTOs ---

// SendMessageRequest is the request body for sending a message.
type SendMessageRequest struct {
	Content string `json:"content"`
}

// SendAIMessageRequest is the request body for POST /rooms/:roomId/messages/ai.
// Content is required. Model is optional and defaults to the
// server-configured model. Private is optional and defaults to false; when
// true, both the resulting human and AI messages are persisted with
// visibility "private" (see MessageResponse.Visibility) and delivered over
// WebSocket only to the requester (private AI mode, phases.md Phase 14).
type SendAIMessageRequest struct {
	Content string `json:"content"`
	Model   string `json:"model"`
	Private bool   `json:"private"`
}

// RegenerateAIMessageRequest is the request body for
// POST /rooms/:roomId/messages/:messageId/regenerate. Model is optional and
// defaults to the server-configured model.
type RegenerateAIMessageRequest struct {
	Model string `json:"model"`
}

// UpdateMessageExcludeRequest is the request body for
// PATCH /rooms/:roomId/messages/:messageId.
//
// ExcludeFromAI is a pointer, not a bare bool: with a bare bool, a caller
// that omits the field entirely (or sends `{}`) would silently decode to
// `false`, un-excluding a message the caller never intended to touch. A
// pointer lets MessageHandler.UpdateExclude distinguish "field omitted"
// (nil) from an explicit `false` and reject the former with HTTP 400.
type UpdateMessageExcludeRequest struct {
	ExcludeFromAI *bool `json:"exclude_from_ai"`
}

// MessageResponse is the JSON response representation of a single message.
// SenderID is nil for system-generated messages. InResponseToMessageID is
// nil for human messages and set to the human message's ID for AI messages.
// IsDeleted is true for a soft-deleted message (see DELETE
// /rooms/:roomId/messages/:messageId); ExcludeFromAI is true when the
// message has been opted out of AI context assembly (see PATCH
// /rooms/:roomId/messages/:messageId). Visibility is "public" (the default)
// or "private"; a "private" message is returned by GET
// /rooms/:roomId/messages only to its own sender (see
// SendAIMessageRequest.Private).
//
// UsedContextSummary reports whether the AI response's context included a
// summary of older room history in place of the raw messages it replaces
// (see usecase/message.MessageUsecase.assembleAIContext, Step 50). It is a
// one-time, request-scoped signal describing how a message was *generated*,
// not a persisted property of the message row: it defaults to false and is
// only ever set to true by the SendAI/RegenerateAI handlers, on the AI
// message they just produced. List/Send/historical reads (and a
// regenerated/refetched view of the same message later) always report
// false.
type MessageResponse struct {
	ID                    string    `json:"id"`
	RoomID                string    `json:"room_id"`
	SenderID              *string   `json:"sender_id"`
	Content               string    `json:"content"`
	Type                  string    `json:"type"`
	Status                string    `json:"status"`
	Sequence              int64     `json:"sequence"`
	InResponseToMessageID *string   `json:"in_response_to_message_id"`
	IsDeleted             bool      `json:"is_deleted"`
	ExcludeFromAI         bool      `json:"exclude_from_ai"`
	Visibility            string    `json:"visibility"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	UsedContextSummary    bool      `json:"used_context_summary"`
}

// SendAIMessageResponse is the response body for POST /rooms/:roomId/messages/ai.
// Always contains both the user message and AI message.
// Check ai_message.status to determine if the LLM call succeeded ("completed") or failed ("failed").
type SendAIMessageResponse struct {
	UserMessage MessageResponse `json:"user_message"`
	AIMessage   MessageResponse `json:"ai_message"`
}

// MessageListResponse is the response body for a paginated list of messages.
// NextCursor is nil when there are no more pages.
type MessageListResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor *string           `json:"next_cursor"`
}

// --- Model DTOs ---

// ModelResponse is the JSON response representation of an available LLM model.
//
// ContextWindow, InputPricePerMillionTokens, and OutputPricePerMillionTokens
// are a flat, pure passthrough of ai.ModelInfo's equivalent fields: 0 means
// "unknown" (the LLM Gateway did not report a value for this model), not
// "no limit"/"free". SupportsImageInput is false for both "no" and
// "unknown", per ai.ModelInfo's documented collapsing of Option<bool>/None.
type ModelResponse struct {
	ID                          string  `json:"id"`
	Name                        string  `json:"name"`
	Provider                    string  `json:"provider"`
	ContextWindow               int     `json:"context_window"`
	InputPricePerMillionTokens  float64 `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens float64 `json:"output_price_per_million_tokens"`
	SupportsImageInput          bool    `json:"supports_image_input"`
}

// ModelListResponse is the response body for GET /models.
type ModelListResponse struct {
	Models []ModelResponse `json:"models"`
}

// --- Token DTOs ---

// ChatMessageDTO is the JSON representation of a single chat message used by
// TokenEstimateRequest.
type ChatMessageDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TokenEstimateRequest is the request body for POST /tokens/estimate. Model
// is required; Messages may be empty (a valid estimation input for an empty
// draft, returning just the fixed overhead).
type TokenEstimateRequest struct {
	Model    string           `json:"model"`
	Messages []ChatMessageDTO `json:"messages"`
}

// TokenEstimateResponse is the response body for POST /tokens/estimate.
// EstimatedTokens is an approximation computed by the LLM Gateway's
// character-based heuristic, not an exact tokenizer count.
type TokenEstimateResponse struct {
	Model           string `json:"model"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// --- User DTOs ---

// UserResponse is the JSON response representation of the authenticated
// caller's own user identity (GET /users/me). It deliberately excludes the
// password hash and any other sensitive credential material.
type UserResponse struct {
	// ID is the user's unique identifier.
	ID string `json:"id"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Username is the user's display/login name.
	Username string `json:"username"`
	// CreatedAt is when the user account was created.
	CreatedAt time.Time `json:"created_at"`
}

// --- Attachment DTOs ---

// PresignUploadRequest is the request body for
// POST /rooms/:roomId/attachments/upload-url. Both fields are required.
type PresignUploadRequest struct {
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

// PresignUploadResponse is the response body for
// POST /rooms/:roomId/attachments/upload-url. UploadURL is a presigned PUT
// URL valid until ExpiresAt; the client uploads object bytes directly to it.
type PresignUploadResponse struct {
	AttachmentID string    `json:"attachment_id"`
	S3Key        string    `json:"s3_key"`
	UploadURL    string    `json:"upload_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// AttachAttachmentRequest is the request body for
// POST /rooms/:roomId/messages/:messageId/attachments.
type AttachAttachmentRequest struct {
	AttachmentID string `json:"attachment_id"`
}

// AttachmentResponse is the JSON response representation of a single
// attachment, without a view URL (returned by the attach endpoint).
type AttachmentResponse struct {
	ID        string    `json:"id"`
	MessageID *string   `json:"message_id"`
	S3Key     string    `json:"s3_key"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// AttachmentViewResponse is the JSON response representation of a single
// attachment including a freshly-presigned view URL (returned by the list
// endpoint).
type AttachmentViewResponse struct {
	ID        string    `json:"id"`
	MessageID *string   `json:"message_id"`
	S3Key     string    `json:"s3_key"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	ViewURL   string    `json:"view_url"`
	CreatedAt time.Time `json:"created_at"`
}

// AttachmentListResponse is the response body for
// GET /rooms/:roomId/messages/:messageId/attachments.
type AttachmentListResponse struct {
	Attachments []AttachmentViewResponse `json:"attachments"`
}

// --- Invitation DTOs ---

// CreateInvitationRequest is the request body for
// POST /rooms/:roomId/invitations. If InviteeUsername is nil, a reusable
// link invitation is created; otherwise it targets that specific user
// (single-use). Role is required and must be one of the five valid
// domainroom.Role values. ExpiresInHours is optional; when omitted it
// defaults to 168 (7 days) and must otherwise fall within [1, 720].
type CreateInvitationRequest struct {
	InviteeUsername *string `json:"invitee_username"`
	Role            string  `json:"role"`
	ExpiresInHours  *int    `json:"expires_in_hours"`
}

// InvitationResponse is the JSON response representation of a single
// invitation. InviteeID is nil for link invitations.
type InvitationResponse struct {
	ID         string    `json:"id"`
	RoomID     string    `json:"room_id"`
	InviterID  string    `json:"inviter_id"`
	InviteeID  *string   `json:"invitee_id"`
	InviteCode string    `json:"invite_code"`
	Role       string    `json:"role"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// InvitationListResponse is the response body for a list of invitations.
type InvitationListResponse struct {
	Invitations []InvitationResponse `json:"invitations"`
}

// RoomMembershipResponse is the JSON response representation of a single
// room membership, returned by POST /invitations/:invitationId/accept.
type RoomMembershipResponse struct {
	ID       string    `json:"id"`
	RoomID   string    `json:"room_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// --- Group DTOs ---

// CreateGroupRequest is the request body for POST /groups. Name is
// required; Description is optional.
type CreateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateGroupRequest is the request body for PUT /groups/:groupId. Name is
// required; Description is optional.
type UpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GroupResponse is the JSON response representation of a single personal
// group.
type GroupResponse struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GroupListResponse is the response body for GET /groups.
type GroupListResponse struct {
	Groups []GroupResponse `json:"groups"`
}

// AddGroupMemberRequest is the request body for
// POST /groups/:groupId/members. Username is required and is resolved to
// an existing user.
type AddGroupMemberRequest struct {
	Username string `json:"username"`
}

// GroupMemberResponse is the JSON response representation of a single
// group membership, with the member's username resolved.
type GroupMemberResponse struct {
	ID       string    `json:"id"`
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	AddedAt  time.Time `json:"added_at"`
}

// GroupMemberListResponse is the response body for
// GET /groups/:groupId/members.
type GroupMemberListResponse struct {
	Members []GroupMemberResponse `json:"members"`
}

// BatchInviteByGroupRequest is the request body for
// POST /rooms/:roomId/invitations/batch-by-group. GroupID and Role are
// required; Role must be one of the four non-"master" domainroom.Role
// values. ExpiresInHours is optional and follows the same
// [1, 720]-hour bounds as CreateInvitationRequest.ExpiresInHours.
type BatchInviteByGroupRequest struct {
	GroupID        string `json:"group_id"`
	Role           string `json:"role"`
	ExpiresInHours *int   `json:"expires_in_hours"`
}

// BatchInviteSkipResponse is the JSON response representation of a single
// group member who was not invited by a batch-invitation call, along with
// the reason (e.g. already a room member, already has a pending
// invitation).
type BatchInviteSkipResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

// BatchInviteByGroupResponse is the response body for
// POST /rooms/:roomId/invitations/batch-by-group. Invited contains every
// invitation successfully created; Skipped contains one entry per group
// member who was not invited, with a reason.
//
// Failed and Error are populated only when the batch was aborted partway
// through by an unexpected (non-skippable) per-member error — see
// groupusecase.GroupUsecase.BatchInviteToRoom's doc comment. In that case
// Invited/Skipped still reflect everything accumulated before the abort
// (an HTTP 500 response with a bare error body would otherwise silently
// discard invitations already created), Failed is true, and Error carries
// the aborting error's message. Both are omitted (zero value) on a fully
// successful batch.
type BatchInviteByGroupResponse struct {
	Invited []InvitationResponse      `json:"invited"`
	Skipped []BatchInviteSkipResponse `json:"skipped"`
	Failed  bool                      `json:"failed,omitempty"`
	Error   string                    `json:"error,omitempty"`
}

// --- Billing DTOs ---

// TokenBalanceResponse is the JSON response representation of a user's
// current token balance, returned by GET /billing/balance. Field names and
// JSON tags match Step 48's web contract (web/src/features/billing/types.ts)
// exactly.
type TokenBalanceResponse struct {
	UserID    string    `json:"user_id"`
	Balance   int64     `json:"balance"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TokenTransactionResponse is the JSON response representation of a single
// token_transactions ledger entry. RoomID is nil for transactions not tied
// to any room (e.g. top-ups). Type is one of "consumption", "charge", or
// "adjustment". Amount is signed: negative for consumption, positive for
// charge/adjustment.
type TokenTransactionResponse struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	RoomID       *string   `json:"room_id"`
	Type         string    `json:"type"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}

// TokenTransactionListResponse is the response body for a paginated list of
// token transactions, returned by GET /billing/transactions. NextCursor is
// nil when there are no more pages.
type TokenTransactionListResponse struct {
	Transactions []TokenTransactionResponse `json:"transactions"`
	NextCursor   *string                    `json:"next_cursor"`
}

// CreateCheckoutSessionRequest is the request body for
// POST /billing/checkout-session. Type is "subscription" (PlanCode
// required) or "token_purchase" (PackageCode required).
type CreateCheckoutSessionRequest struct {
	Type        string `json:"type"`
	PlanCode    string `json:"plan_code,omitempty"`
	PackageCode string `json:"package_code,omitempty"`
}

// CheckoutSessionResponse is the response body for
// POST /billing/checkout-session: a Stripe-hosted Checkout page URL to
// redirect the user to.
type CheckoutSessionResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

// BillingPortalRequest is the request body for POST /billing/portal-session.
type BillingPortalRequest struct {
	ReturnURL string `json:"return_url"`
}

// BillingPortalResponse is the response body for
// POST /billing/portal-session: a Stripe-hosted Billing Portal URL.
type BillingPortalResponse struct {
	PortalURL string `json:"portal_url"`
}

// SubscriptionResponse is the JSON response representation of a user's
// Subscription, returned by GET /billing/subscription and
// POST /billing/subscription/cancel. Field names match Step 53's web
// contract exactly. CanceledAt is nil until the subscription has actually
// ended (see billing.Subscription's CancelAtPeriodEnd/CanceledAt doc).
type SubscriptionResponse struct {
	Status                 string     `json:"status"`
	PlanCode               string     `json:"plan_code"`
	MonthlyTokenAllocation int64      `json:"monthly_token_allocation"`
	CurrentPeriodStart     time.Time  `json:"current_period_start"`
	CurrentPeriodEnd       time.Time  `json:"current_period_end"`
	CancelAtPeriodEnd      bool       `json:"cancel_at_period_end"`
	CanceledAt             *time.Time `json:"canceled_at"`
}

// PaymentRecordResponse is the JSON response representation of a single
// payment_history row. StripeReferenceID is the Stripe Checkout Session ID
// for a token_purchase row (see PaymentRecord.StripeReferenceID) — the web
// checkout success page matches it against the "session_id" query parameter
// Stripe's redirect carries to confirm which specific purchase completed,
// rather than inferring completion from a balance delta.
type PaymentRecordResponse struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	AmountCents       int64     `json:"amount_cents"`
	Currency          string    `json:"currency"`
	TokensCredited    int64     `json:"tokens_credited"`
	Status            string    `json:"status"`
	StripeReferenceID string    `json:"stripe_reference_id"`
	CreatedAt         time.Time `json:"created_at"`
}

// PaymentHistoryResponse is the response body for a paginated list of
// payment records, returned by GET /billing/payments. NextCursor is nil
// when there are no more pages.
type PaymentHistoryResponse struct {
	Payments   []PaymentRecordResponse `json:"payments"`
	NextCursor *string                 `json:"next_cursor"`
}

// BillingPlanResponse is a single entry of the purchasable catalog served by
// GET /billing/plans. Interval is "month" for a subscription plan or
// "one_time" for a token package. stripe_price_id is deliberately not
// exposed here.
type BillingPlanResponse struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	PriceCents     int64  `json:"price_cents"`
	Currency       string `json:"currency"`
	Interval       string `json:"interval"`
	TokenAllowance int64  `json:"token_allowance"`
}

// BillingPlanListResponse is the response body for GET /billing/plans.
type BillingPlanListResponse struct {
	Plans []BillingPlanResponse `json:"plans"`
}

// --- Common DTOs ---

// ErrorResponse is the standard error response body used across all handler
// error responses. It contains a human-readable error message.
type ErrorResponse struct {
	Message string `json:"message"`
}
