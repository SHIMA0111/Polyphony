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
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		SenderID:  &rm.OwnerID,
		Content:   "first",
		Type:      domainmessage.MessageTypeHuman,
		Status:    domainmessage.MessageStatusCompleted,
		Sequence:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := msgRepo.Create(ctx, first); err != nil {
		t.Fatalf("create first message: %v", err)
	}

	second := &domainmessage.Message{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		SenderID:  &rm.OwnerID,
		Content:   "second, same sequence",
		Type:      domainmessage.MessageTypeHuman,
		Status:    domainmessage.MessageStatusCompleted,
		Sequence:  1, // duplicate room_id + sequence
		CreatedAt: now,
		UpdatedAt: now,
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
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		SenderID:  &rm.OwnerID,
		Content:   "What is Go?",
		Type:      domainmessage.MessageTypeHuman,
		Status:    domainmessage.MessageStatusCompleted,
		Sequence:  1,
		CreatedAt: now,
		UpdatedAt: now,
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
		InResponseToMessageID: &humanMsg.ID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := msgRepo.Create(ctx, aiMsg); err != nil {
		t.Fatalf("create AI message: %v", err)
	}

	gotHuman, err := msgRepo.GetByID(ctx, humanMsg.ID)
	if err != nil {
		t.Fatalf("get human message: %v", err)
	}
	if gotHuman.InResponseToMessageID != nil {
		t.Fatalf("expected human message InResponseToMessageID to be nil, got %v", *gotHuman.InResponseToMessageID)
	}

	gotAI, err := msgRepo.GetByID(ctx, aiMsg.ID)
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
