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

// RoomResponse is the response body for a room.
type RoomResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

// MessageResponse is the JSON response representation of a single message.
// SenderID is nil for system-generated messages. InResponseToMessageID is
// nil for human messages and set to the human message's ID for AI messages.
type MessageResponse struct {
	ID                    string    `json:"id"`
	RoomID                string    `json:"room_id"`
	SenderID              *string   `json:"sender_id"`
	Content               string    `json:"content"`
	Type                  string    `json:"type"`
	Status                string    `json:"status"`
	Sequence              int64     `json:"sequence"`
	InResponseToMessageID *string   `json:"in_response_to_message_id"`
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

// --- User DTOs ---

// UserResponse is the JSON response representation of the authenticated
// caller's own user identity (GET /users/me). It deliberately excludes the
// password hash and any other sensitive credential material.
type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
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

// --- Common DTOs ---

// ErrorResponse is the standard error response body used across all handler
// error responses. It contains a human-readable error message.
type ErrorResponse struct {
	Message string `json:"message"`
}
