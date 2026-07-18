//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// seedUserAndRoom creates a user and a room owned by that user, returning
// the room. It is a small shared setup helper for the message repository
// integration tests in this file.
func seedUserAndRoom(ctx context.Context, t *testing.T, userRepo *UserRepository, roomRepo *RoomRepository, label string) *domainroom.Room {
	t.Helper()

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        label + "@example.com",
		Username:     label,
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        label + " room",
		Description: "",
		OwnerID:     owner.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("create room: %v", err)
	}
	return rm
}

// TestMessages_UniqueRoomSequence proves that the
// messages_room_sequence_unique UNIQUE(room_id, sequence) constraint added
// to schema.sql is enforced by the database: inserting two messages with
// the same room_id and sequence must fail on the second insert with a
// unique-violation error, independent of application-level sequence
// allocation.
func TestMessages_UniqueRoomSequence(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "unique-seq-owner")

	now := time.Now()
	first := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "first",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, first); err != nil {
		t.Fatalf("create first message: %v", err)
	}

	second := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "second, same sequence",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1, // duplicate room_id + sequence
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	err := msgRepo.Create(ctx, second)
	if err == nil {
		t.Fatal("expected the second insert with a duplicate (room_id, sequence) to fail, got nil error")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected a *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != pgerrcode.UniqueViolation {
		t.Fatalf("expected unique_violation (%s), got code %s: %v", pgerrcode.UniqueViolation, pgErr.Code, err)
	}
	if pgErr.ConstraintName != "messages_room_sequence_unique" {
		t.Fatalf("expected constraint messages_room_sequence_unique, got %s", pgErr.ConstraintName)
	}
}

// TestMessageRepository_InResponseToMessageIDRoundTrip proves that
// in_response_to_message_id round-trips correctly through Create and
// scanMessage: nil for a human message, and set to the referenced human
// message's ID for an AI message.
func TestMessageRepository_InResponseToMessageIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "response-link-owner")

	now := time.Now()
	humanMsg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "What is Go?",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, humanMsg); err != nil {
		t.Fatalf("create human message: %v", err)
	}

	aiMsg := &domainmessage.Message{
		ID:                    uuid.New().String(),
		RoomID:                rm.ID,
		SenderID:              nil,
		Content:               "Go is a programming language.",
		Type:                  domainmessage.MessageTypeAI,
		Status:                domainmessage.MessageStatusCompleted,
		Sequence:              2,
		Visibility:            domainmessage.MessageVisibilityPublic,
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := msgRepo.Create(ctx, aiMsg); err != nil {
		t.Fatalf("create AI message: %v", err)
	}

	gotHuman, err := msgRepo.GetByID(ctx, humanMsg.ID, rm.OwnerID)
	if err != nil {
		t.Fatalf("get human message: %v", err)
	}
	if gotHuman.InResponseToMessageID != nil {
		t.Fatalf("expected human message InResponseToMessageID to be nil, got %v", *gotHuman.InResponseToMessageID)
	}

	gotAI, err := msgRepo.GetByID(ctx, aiMsg.ID, rm.OwnerID)
	if err != nil {
		t.Fatalf("get AI message: %v", err)
	}
	if gotAI.InResponseToMessageID == nil {
		t.Fatal("expected AI message InResponseToMessageID to be set")
	}
	if *gotAI.InResponseToMessageID != humanMsg.ID {
		t.Fatalf("expected AI message InResponseToMessageID == %s, got %s", humanMsg.ID, *gotAI.InResponseToMessageID)
	}
}

// TestMessageRepository_PrivateVisibilityFiltering proves the SQL-level
// visibility predicate applied by GetByID and ListByRoom (see
// visibilityFilter): a private message is returned only when
// requestingUserID matches its sender_id, and is otherwise excluded exactly
// as if it did not exist — for both a single-row GetByID lookup and a
// room-wide ListByRoom page.
func TestMessageRepository_PrivateVisibilityFiltering(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "private-vis-owner")

	other := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "private-vis-other@example.com",
		Username:     "private-vis-other",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, other); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	now := time.Now()
	privateMsg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "a private question",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPrivate,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, privateMsg); err != nil {
		t.Fatalf("create private message: %v", err)
	}

	// GetByID: invisible to a non-owner (surfaced as ErrNotFound, identical
	// to a genuinely missing row), visible to the owner.
	if _, err := msgRepo.GetByID(ctx, privateMsg.ID, other.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-owner GetByID of a private message, got %v", err)
	}
	got, err := msgRepo.GetByID(ctx, privateMsg.ID, rm.OwnerID)
	if err != nil {
		t.Fatalf("expected owner GetByID of a private message to succeed, got error: %v", err)
	}
	if got.ID != privateMsg.ID {
		t.Fatalf("expected to get the private message back, got %s", got.ID)
	}

	// ListByRoom: the private message is excluded from a non-owner's page
	// but included in the owner's page.
	otherPage, err := msgRepo.ListByRoom(ctx, rm.ID, "", 20, other.ID)
	if err != nil {
		t.Fatalf("ListByRoom (non-owner) failed: %v", err)
	}
	for _, m := range otherPage.Messages {
		if m.ID == privateMsg.ID {
			t.Fatal("expected non-owner ListByRoom to exclude the private message")
		}
	}

	ownerPage, err := msgRepo.ListByRoom(ctx, rm.ID, "", 20, rm.OwnerID)
	if err != nil {
		t.Fatalf("ListByRoom (owner) failed: %v", err)
	}
	found := false
	for _, m := range ownerPage.Messages {
		if m.ID == privateMsg.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected owner ListByRoom to include the private message")
	}

	// ListByRoomUpTo: same owner/non-owner visibility rule, exercised
	// directly rather than only indirectly through ListByRoom/GetByID above.
	// This is the query the AI context builder (SendAIMessage/
	// RegenerateAIMessage's context-assembly step) actually calls, so it
	// needs its own direct coverage of the private-visibility filter rather
	// than relying on ListByRoom's coverage to stand in for it.
	otherUpTo, err := msgRepo.ListByRoomUpTo(ctx, rm.ID, privateMsg.Sequence, 20, other.ID)
	if err != nil {
		t.Fatalf("ListByRoomUpTo (non-owner) failed: %v", err)
	}
	for _, m := range otherUpTo {
		if m.ID == privateMsg.ID {
			t.Fatal("expected non-owner ListByRoomUpTo to exclude the private message")
		}
	}

	ownerUpTo, err := msgRepo.ListByRoomUpTo(ctx, rm.ID, privateMsg.Sequence, 20, rm.OwnerID)
	if err != nil {
		t.Fatalf("ListByRoomUpTo (owner) failed: %v", err)
	}
	foundUpTo := false
	for _, m := range ownerUpTo {
		if m.ID == privateMsg.ID {
			foundUpTo = true
		}
	}
	if !foundUpTo {
		t.Fatal("expected owner ListByRoomUpTo to include the private message")
	}
}

// TestMessageRepository_PrivateCursorMatchesUnknownCursor proves that
// ListByRoom's cursor-resolution subquery applies the same visibilityFilter
// as the surrounding list queries: a non-owner using another user's private
// message ID as the cursor must get domain.ErrNotFound, indistinguishable
// from passing a cursor ID that does not exist at all. Without this, a
// non-owner could use a private message's ID as a cursor to confirm its
// existence (and its position in the sequence) purely from the presence or
// absence of an error, even though that same message is correctly excluded
// from every listed page.
func TestMessageRepository_PrivateCursorMatchesUnknownCursor(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "private-cursor-owner")

	other := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "private-cursor-other@example.com",
		Username:     "private-cursor-other",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, other); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	now := time.Now()
	privateMsg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "a private cursor target",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPrivate,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, privateMsg); err != nil {
		t.Fatalf("create private message: %v", err)
	}
	// A second, later message so a successful cursor page would have
	// something to return -- if the visibility filter were missing, the
	// non-owner's ListByRoom call below would succeed and (incorrectly)
	// page starting after the private message's sequence.
	second := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "a later public message",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   2,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, second); err != nil {
		t.Fatalf("create second message: %v", err)
	}

	// Baseline: an unknown cursor ID (never inserted) yields ErrNotFound.
	_, unknownErr := msgRepo.ListByRoom(ctx, rm.ID, uuid.New().String(), 20, other.ID)
	if !errors.Is(unknownErr, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown cursor, got %v", unknownErr)
	}

	// A non-owner using the private message's real ID as the cursor must
	// get the identical error.
	_, privateErr := msgRepo.ListByRoom(ctx, rm.ID, privateMsg.ID, 20, other.ID)
	if !errors.Is(privateErr, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a non-owner's private-message cursor, got %v", privateErr)
	}

	// The owner, by contrast, can use the same ID as a cursor successfully.
	ownerPage, err := msgRepo.ListByRoom(ctx, rm.ID, privateMsg.ID, 20, rm.OwnerID)
	if err != nil {
		t.Fatalf("expected owner cursor resolution to succeed, got error: %v", err)
	}
	if len(ownerPage.Messages) != 0 {
		t.Fatalf("expected no messages before sequence 1, got %d", len(ownerPage.Messages))
	}
}
