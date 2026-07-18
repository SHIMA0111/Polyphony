// Package room defines the room and membership entities and their repository port.
package room

import "time"

// Room represents a chat room where users and AI interact.
type Room struct {
	ID          string
	Name        string
	Description string
	OwnerID     string
	// AIContextCutoffAt, when non-nil, is the earliest CreatedAt an AI
	// context message may have: messages created strictly before this
	// timestamp are excluded from ai.ContextBuilder.Build's output. A nil
	// value means no cutoff is configured — the room's entire eligible
	// history is considered.
	AIContextCutoffAt *time.Time
	// AIProvider is the room's configured default LLM provider (e.g.
	// "anthropic", "openai"), stored for display/consistency alongside
	// AIModel. It is not consulted by model resolution (see
	// usecase/message.resolveModel), which resolves only a model string. A
	// nil value means "not configured".
	AIProvider *string
	// AIModel is the room's configured default model string (e.g.
	// "claude-opus-4"), used by usecase/message.resolveModel as the
	// second-highest precedence tier (below an explicit per-request model,
	// above the deployment-wide Config.DefaultAIModel). A nil or empty
	// value means "not configured" — resolution falls through to the next
	// tier.
	AIModel   *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RoomMember represents a user's membership in a room, including their
// authorization Role within it.
type RoomMember struct {
	ID       string
	RoomID   string
	UserID   string
	Role     Role
	JoinedAt time.Time
	// Username is the member's display username, populated by
	// RoomRepository.ListMembers and RoomRepository.GetMember (both JOIN
	// against the users table). It is left empty ("") only on RoomMember
	// values that were never round-tripped through the database with a
	// users JOIN -- e.g. the struct AddMember's caller constructs before
	// calling it, or values returned by other write-oriented repository
	// methods that have no reason to look up the username at all.
	Username string
}

// RoomWithRole pairs a Room with a specific user's Role in that room. It is
// returned by RoomRepository.ListByUserIDWithRole (and used by the usecase
// layer as the return type of GetRoom/CreateRoom/UpdateRoom/ListRooms) so
// callers can surface the requesting user's permission level without a
// second repository round-trip.
type RoomWithRole struct {
	Room *Room
	Role Role
}
