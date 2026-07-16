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

// TestMessageRepository_CountByRoom proves CountByRoom returns the total
// number of messages in a room, ignoring soft-delete/visibility.
func TestMessageRepository_CountByRoom(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "count-by-room-owner")

	count, err := msgRepo.CountByRoom(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountByRoom (empty room) failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 messages in a fresh room, got %d", count)
	}

	now := time.Now()
	for i := int64(1); i <= 3; i++ {
		msg := &domainmessage.Message{
			ID:         uuid.New().String(),
			RoomID:     rm.ID,
			SenderID:   &rm.OwnerID,
			Content:    "msg",
			Type:       domainmessage.MessageTypeHuman,
			Status:     domainmessage.MessageStatusCompleted,
			Sequence:   i,
			Visibility: domainmessage.MessageVisibilityPublic,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := msgRepo.Create(ctx, msg); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	count, err = msgRepo.CountByRoom(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountByRoom failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 messages, got %d", count)
	}

	// Soft-deleting one message must not change the count (CountByRoom is a
	// structural count, unlike ListByRoom/ListByRoomUpTo).
	var firstID string
	if err := pool.QueryRow(ctx, `SELECT id FROM messages WHERE room_id = $1 AND sequence = 1`, rm.ID).Scan(&firstID); err != nil {
		t.Fatalf("query first message id: %v", err)
	}
	if err := msgRepo.Delete(ctx, firstID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	count, err = msgRepo.CountByRoom(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountByRoom after delete failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected CountByRoom to still be 3 after a soft-delete, got %d", count)
	}
}

// TestMessageRepository_ListByRoomAfter proves ListByRoomAfter returns
// messages strictly after afterSequence, in ascending order, respecting
// limit — the oldest-first counterpart to ListByRoomUpTo.
func TestMessageRepository_ListByRoomAfter(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "list-after-owner")

	now := time.Now()
	var ids []string
	for i := int64(1); i <= 5; i++ {
		msg := &domainmessage.Message{
			ID:         uuid.New().String(),
			RoomID:     rm.ID,
			SenderID:   &rm.OwnerID,
			Content:    "msg",
			Type:       domainmessage.MessageTypeHuman,
			Status:     domainmessage.MessageStatusCompleted,
			Sequence:   i,
			Visibility: domainmessage.MessageVisibilityPublic,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := msgRepo.Create(ctx, msg); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		ids = append(ids, msg.ID)
	}

	// afterSequence=2, limit=2 should return sequences 3 and 4, ascending.
	page, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 2, 2)
	if err != nil {
		t.Fatalf("ListByRoomAfter failed: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page))
	}
	if page[0].Sequence != 3 || page[1].Sequence != 4 {
		t.Fatalf("expected sequences [3 4] ascending, got [%d %d]", page[0].Sequence, page[1].Sequence)
	}
	if page[0].ID != ids[2] || page[1].ID != ids[3] {
		t.Fatal("expected IDs to match the messages created at sequences 3 and 4")
	}

	// afterSequence=0 with a limit larger than the room's message count
	// returns everything, still ascending.
	all, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 0, 100)
	if err != nil {
		t.Fatalf("ListByRoomAfter (all) failed: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(all))
	}
	for i, m := range all {
		if m.Sequence != int64(i+1) {
			t.Fatalf("expected ascending sequence %d at index %d, got %d", i+1, i, m.Sequence)
		}
	}

	// afterSequence beyond the last message returns an empty slice (the
	// fork worker's loop-termination condition).
	empty, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 5, 100)
	if err != nil {
		t.Fatalf("ListByRoomAfter (beyond end) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 messages after the last sequence, got %d", len(empty))
	}
}

// TestMessageRepository_CreateBatch proves CreateBatch persists every
// message in a single transaction and rolls back entirely on a failure
// partway through (no partial batch persisted).
func TestMessageRepository_CreateBatch(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "create-batch-owner")

	now := time.Now()
	batch := make([]*domainmessage.Message, 4)
	for i := range batch {
		batch[i] = &domainmessage.Message{
			ID:         uuid.New().String(),
			RoomID:     rm.ID,
			SenderID:   &rm.OwnerID,
			Content:    "batched",
			Type:       domainmessage.MessageTypeHuman,
			Status:     domainmessage.MessageStatusCompleted,
			Sequence:   int64(i + 1),
			Visibility: domainmessage.MessageVisibilityPublic,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	if err := msgRepo.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	count, err := msgRepo.CountByRoom(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountByRoom failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 messages after CreateBatch, got %d", count)
	}

	// A batch with a duplicate (room_id, sequence) partway through must
	// roll back entirely: none of the batch's messages should be persisted,
	// including the ones before the conflicting entry.
	failingBatch := []*domainmessage.Message{
		{
			ID: uuid.New().String(), RoomID: rm.ID, SenderID: &rm.OwnerID, Content: "ok",
			Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted,
			Sequence: 100, Visibility: domainmessage.MessageVisibilityPublic, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: uuid.New().String(), RoomID: rm.ID, SenderID: &rm.OwnerID, Content: "duplicate sequence",
			Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted,
			Sequence: 1, Visibility: domainmessage.MessageVisibilityPublic, CreatedAt: now, UpdatedAt: now, // conflicts with batch[0]
		},
	}
	if err := msgRepo.CreateBatch(ctx, failingBatch); err == nil {
		t.Fatal("expected CreateBatch to fail on a duplicate (room_id, sequence), got nil")
	}

	count, err = msgRepo.CountByRoom(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountByRoom after failed batch failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected CreateBatch's failure to roll back entirely (still 4 messages), got %d", count)
	}
}
