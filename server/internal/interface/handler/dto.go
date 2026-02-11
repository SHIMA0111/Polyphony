package handler

import "time"

// --- Auth DTOs ---

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResponse is the response body containing an access token.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// --- Room DTOs ---

// CreateRoomRequest is the request body for creating a room.
type CreateRoomRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateRoomRequest is the request body for updating a room.
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

// SendAIMessageRequest is the request body for sending a message and getting an AI response.
type SendAIMessageRequest struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

// MessageResponse is the response body for a message.
type MessageResponse struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	SenderID  *string   `json:"sender_id"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`
	Sequence  int64     `json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageListResponse is the response body for a paginated list of messages.
type MessageListResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor *string           `json:"next_cursor"`
}

// --- Common DTOs ---

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Message string `json:"message"`
}
