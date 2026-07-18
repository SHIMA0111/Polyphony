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
// (Config.DefaultAIModel).
type RoomResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	OwnerID           string     `json:"owner_id"`
	Role              string     `json:"role"`
	AIContextCutoffAt *time.Time `json:"ai_context_cutoff_at"`
	AIProvider        *string    `json:"ai_provider"`
	AIModel           *string    `json:"ai_model"`
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

// --- Message DTOs ---

// SendMessageRequest is the request body for sending a message.
type SendMessageRequest struct {
	Content string `json:"content"`
}

// SendAIMessageRequest is the request body for POST /rooms/:roomId/messages/ai.
// Content is required. Model is optional and defaults to the server-configured model.
type SendAIMessageRequest struct {
	Content string `json:"content"`
	Model   string `json:"model"`
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
// /rooms/:roomId/messages/:messageId).
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
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
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
type ModelResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
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

// --- Common DTOs ---

// ErrorResponse is the standard error response body used across all handler
// error responses. It contains a human-readable error message.
type ErrorResponse struct {
	Message string `json:"message"`
}
