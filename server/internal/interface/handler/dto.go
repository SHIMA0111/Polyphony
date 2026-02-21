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
// SenderID is nil for system-generated messages.
type MessageResponse struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	SenderID  *string   `json:"sender_id"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Sequence  int64     `json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// --- Common DTOs ---

// ErrorResponse is the standard error response body used across all handler
// error responses. It contains a human-readable error message.
type ErrorResponse struct {
	Message string `json:"message"`
}
