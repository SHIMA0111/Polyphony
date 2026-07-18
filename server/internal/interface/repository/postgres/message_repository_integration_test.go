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
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
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

	// ListByRoomUpTo: same private-message inclusion/exclusion contract as
	// ListByRoom above, asserted directly rather than only indirectly via
	// ListByRoom, since ListByRoomUpTo is what feeds
	// MessageUsecase.assembleAIContext's AI-context-window fetch (see
	// server/internal/usecase/message.usecase.go) and had no direct test
	// coverage of its own.
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

// TestMessageRepository_ListByRoomAfter proves ListByRoomAfter returns
// messages strictly after afterSequence, in ascending order, respecting
// limit and maxSequence — the oldest-first counterpart to ListByRoomUpTo —
// and that it is a deliberately unfiltered structural read: soft-deleted,
// private-visibility, and AI-excluded rows all come back with their flags
// intact rather than being scrubbed or dropped, since this is exactly what
// the room-fork copy job (usecase/room.RoomUsecase.runForkJob) relies on to
// carry every message over verbatim, unlike ListByRoom/ListByRoomUpTo/AI
// context assembly, which all filter on these same flags.
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
		if i == 4 {
			// Visibility is honored directly by Create (unlike
			// is_deleted/exclude_from_ai below, which Create always inserts
			// as false regardless of the struct's field values).
			msg.Visibility = domainmessage.MessageVisibilityPrivate
		}
		if err := msgRepo.Create(ctx, msg); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		ids = append(ids, msg.ID)
	}

	// Soft-delete sequence 3 and exclude sequence 5 from AI context via
	// their own dedicated repository calls (Create cannot seed either flag
	// directly — see above).
	if err := msgRepo.Delete(ctx, ids[2]); err != nil {
		t.Fatalf("soft-delete message 3: %v", err)
	}
	if err := msgRepo.UpdateExcludeFromAI(ctx, ids[4], true, now); err != nil {
		t.Fatalf("exclude message 5 from AI: %v", err)
	}

	// afterSequence=2, maxSequence=100 (unbounded relative to this room's 5
	// messages), limit=2 should return sequences 3 and 4, ascending, with
	// the soft-delete/private flags on those rows intact.
	page, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 2, 100, 2)
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
	if !page[0].IsDeleted {
		t.Fatal("expected the soft-deleted message at sequence 3 to remain included with IsDeleted set")
	}
	if page[1].Visibility != domainmessage.MessageVisibilityPrivate {
		t.Fatalf("expected the private message at sequence 4 to remain included with its visibility intact, got %q", page[1].Visibility)
	}

	// afterSequence=0 with a limit larger than the room's message count
	// returns everything, still ascending, including the AI-excluded
	// message at sequence 5 with its flag intact.
	all, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 0, 100, 100)
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
	if !all[4].ExcludeFromAI {
		t.Fatal("expected the AI-excluded message at sequence 5 to remain included with ExcludeFromAI set")
	}

	// afterSequence beyond the last message returns an empty slice (the
	// fork worker's loop-termination condition).
	empty, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 5, 100, 100)
	if err != nil {
		t.Fatalf("ListByRoomAfter (beyond end) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 messages after the last sequence, got %d", len(empty))
	}

	// maxSequence caps the result even when limit alone would return more —
	// this is the frozen upper bound a room-fork job passes to every
	// ListByRoomAfter call across its copy (see
	// usecase/room.RoomUsecase.runForkJob), so a message inserted after the
	// job's initial CountAndMaxSequence snapshot is excluded from the copy
	// entirely. The soft-deleted message at sequence 3 falls within this
	// bound and must still be included.
	capped, err := msgRepo.ListByRoomAfter(ctx, rm.ID, 0, 3, 100)
	if err != nil {
		t.Fatalf("ListByRoomAfter (maxSequence-capped) failed: %v", err)
	}
	if len(capped) != 3 {
		t.Fatalf("expected 3 messages capped at maxSequence=3, got %d", len(capped))
	}
	for i, m := range capped {
		if m.Sequence != int64(i+1) {
			t.Fatalf("expected ascending sequence %d at index %d, got %d", i+1, i, m.Sequence)
		}
	}
	if !capped[2].IsDeleted {
		t.Fatal("expected the soft-deleted message at sequence 3 to remain included within the maxSequence bound")
	}
}

// TestMessageRepository_CountAndMaxSequence proves CountAndMaxSequence
// returns the total message count and the highest sequence value in a
// single atomic read, ignoring soft-delete/visibility (a structural read,
// not a visibility-filtered one), and reports (0, 0) for a room with no
// messages.
func TestMessageRepository_CountAndMaxSequence(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "count-and-max-seq-owner")

	total, maxSeq, err := msgRepo.CountAndMaxSequence(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence (empty room) failed: %v", err)
	}
	if total != 0 || maxSeq != 0 {
		t.Fatalf("expected (0, 0) for a fresh room, got (%d, %d)", total, maxSeq)
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

	total, maxSeq, err = msgRepo.CountAndMaxSequence(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence failed: %v", err)
	}
	if total != 3 || maxSeq != 3 {
		t.Fatalf("expected (3, 3), got (%d, %d)", total, maxSeq)
	}

	// Soft-deleting the highest-sequence message must not change either
	// value (CountAndMaxSequence is a structural read, unlike
	// ListByRoom/ListByRoomUpTo).
	var lastID string
	if err := pool.QueryRow(ctx, `SELECT id FROM messages WHERE room_id = $1 AND sequence = 3`, rm.ID).Scan(&lastID); err != nil {
		t.Fatalf("query last message id: %v", err)
	}
	if err := msgRepo.Delete(ctx, lastID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	total, maxSeq, err = msgRepo.CountAndMaxSequence(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence after delete failed: %v", err)
	}
	if total != 3 || maxSeq != 3 {
		t.Fatalf("expected CountAndMaxSequence to still be (3, 3) after a soft-delete, got (%d, %d)", total, maxSeq)
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

	count, _, err := msgRepo.CountAndMaxSequence(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence failed: %v", err)
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

	count, _, err = msgRepo.CountAndMaxSequence(ctx, rm.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence after failed batch failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected CreateBatch's failure to roll back entirely (still 4 messages), got %d", count)
	}
}

// TestMessageRepository_ListByRoomCursorVisibility proves that ListByRoom's
// cursor-resolution subquery applies the same visibilityFilter as the
// surrounding list queries: a non-owner using another user's private
// message ID as a cursor gets domain.ErrNotFound identical to using an
// unknown cursor, instead of the cursor silently resolving against a row
// the caller cannot otherwise see.
func TestMessageRepository_ListByRoomCursorVisibility(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "cursor-vis-owner")

	other := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "cursor-vis-other@example.com",
		Username:     "cursor-vis-other",
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

	// A trailing public message so the room is non-empty for the owner's
	// cursor-based page below.
	publicMsg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "a public message",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   2,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, publicMsg); err != nil {
		t.Fatalf("create public message: %v", err)
	}

	// Baseline: an unknown cursor yields a wrapped domain.ErrNotFound.
	_, unknownErr := msgRepo.ListByRoom(ctx, rm.ID, uuid.New().String(), 20, other.ID)
	if !errors.Is(unknownErr, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown cursor, got %v", unknownErr)
	}

	// A non-owner using the private message's ID as the cursor must get the
	// identical error, not a resolved page.
	_, privateErr := msgRepo.ListByRoom(ctx, rm.ID, privateMsg.ID, 20, other.ID)
	if !errors.Is(privateErr, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a non-owner's private-message cursor, got %v", privateErr)
	}

	// The owner, by contrast, can use the private message as a cursor: it is
	// visible to them, so the page resolves normally.
	ownerPage, err := msgRepo.ListByRoom(ctx, rm.ID, privateMsg.ID, 20, rm.OwnerID)
	if err != nil {
		t.Fatalf("expected owner cursor lookup on the private message to succeed, got error: %v", err)
	}
	if len(ownerPage.Messages) != 0 {
		t.Fatalf("expected no messages older than sequence 1, got %d", len(ownerPage.Messages))
	}
}

// TestMessageRepository_DeleteAndInvalidateSummary proves that
// DeleteAndInvalidateSummary soft-deletes the message and invalidates the
// room's cached context summary (a subsequent ContextSummaryRepository.Get
// returns domain.ErrNotFound, and the invalidation revision advances) in one
// call — see domainmessage.MessageRepository.DeleteAndInvalidateSummary's
// doc comment for why this must be atomic.
func TestMessageRepository_DeleteAndInvalidateSummary(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "delete-invalidate-owner")

	now := time.Now()
	msg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "to be deleted",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 1,
		SummaryText:         "cached summary that should be invalidated",
		TokenCount:          10,
	}, 0); err != nil {
		t.Fatalf("seed cached summary: %v", err)
	}

	if err := msgRepo.DeleteAndInvalidateSummary(ctx, msg.ID, rm.ID); err != nil {
		t.Fatalf("DeleteAndInvalidateSummary failed: %v", err)
	}

	got, err := msgRepo.GetByID(ctx, msg.ID, rm.OwnerID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !got.IsDeleted {
		t.Fatal("expected the message to be soft-deleted")
	}

	if _, err := summaryRepo.Get(ctx, rm.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the cached summary to be invalidated, got err=%v", err)
	}
	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("expected the invalidation revision to advance to 1, got %d", revision)
	}
}

// TestMessageRepository_DeleteAndInvalidateSummaryNotFound proves that
// DeleteAndInvalidateSummary returns domain.ErrNotFound for a nonexistent
// (or already soft-deleted) message without ever attempting the summary
// invalidation step — a cached summary and its revision are left completely
// untouched, since the real implementation's mutation statement runs (and
// is checked for zero rows affected) before the summary step, and the whole
// transaction rolls back on that ErrNotFound.
func TestMessageRepository_DeleteAndInvalidateSummaryNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "delete-invalidate-notfound-owner")

	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 1,
		SummaryText:         "must survive the failed delete",
		TokenCount:          10,
	}, 0); err != nil {
		t.Fatalf("seed cached summary: %v", err)
	}

	err := msgRepo.DeleteAndInvalidateSummary(ctx, uuid.New().String(), rm.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a nonexistent message, got %v", err)
	}

	got, err := summaryRepo.Get(ctx, rm.ID)
	if err != nil {
		t.Fatalf("expected the cached summary to survive a failed delete, got err=%v", err)
	}
	if got.SummaryText != "must survive the failed delete" {
		t.Fatalf("expected the cached summary to be unchanged, got %q", got.SummaryText)
	}
	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 0 {
		t.Fatalf("expected the invalidation revision to remain 0, got %d", revision)
	}
}

// TestMessageRepository_UpdateExcludeFromAIAndInvalidateSummary proves that
// UpdateExcludeFromAIAndInvalidateSummary toggles exclude_from_ai and
// invalidates the room's cached context summary in one call — the
// exclude_from_ai counterpart to
// TestMessageRepository_DeleteAndInvalidateSummary.
func TestMessageRepository_UpdateExcludeFromAIAndInvalidateSummary(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "exclude-invalidate-owner")

	now := time.Now()
	msg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     rm.ID,
		SenderID:   &rm.OwnerID,
		Content:    "to be excluded",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   1,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 1,
		SummaryText:         "cached summary that should be invalidated",
		TokenCount:          10,
	}, 0); err != nil {
		t.Fatalf("seed cached summary: %v", err)
	}

	updatedAt := now.Add(time.Minute)
	if err := msgRepo.UpdateExcludeFromAIAndInvalidateSummary(ctx, msg.ID, rm.ID, true, updatedAt); err != nil {
		t.Fatalf("UpdateExcludeFromAIAndInvalidateSummary failed: %v", err)
	}

	got, err := msgRepo.GetByID(ctx, msg.ID, rm.OwnerID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !got.ExcludeFromAI {
		t.Fatal("expected ExcludeFromAI to be true")
	}

	if _, err := summaryRepo.Get(ctx, rm.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected the cached summary to be invalidated, got err=%v", err)
	}
	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("expected the invalidation revision to advance to 1, got %d", revision)
	}
}

// TestMessageRepository_UpdateExcludeFromAIAndInvalidateSummaryNotFound
// proves that UpdateExcludeFromAIAndInvalidateSummary returns
// domain.ErrNotFound for a nonexistent message without attempting the
// summary invalidation step, mirroring
// TestMessageRepository_DeleteAndInvalidateSummaryNotFound.
func TestMessageRepository_UpdateExcludeFromAIAndInvalidateSummaryNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	summaryRepo := NewContextSummaryRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "exclude-invalidate-notfound-owner")

	if err := summaryRepo.Upsert(ctx, &ai.ContextSummary{
		RoomID:              rm.ID,
		Model:               "gpt-5-mini",
		CoveredUpToSequence: 1,
		SummaryText:         "must survive the failed update",
		TokenCount:          10,
	}, 0); err != nil {
		t.Fatalf("seed cached summary: %v", err)
	}

	err := msgRepo.UpdateExcludeFromAIAndInvalidateSummary(ctx, uuid.New().String(), rm.ID, true, time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a nonexistent message, got %v", err)
	}

	got, err := summaryRepo.Get(ctx, rm.ID)
	if err != nil {
		t.Fatalf("expected the cached summary to survive a failed update, got err=%v", err)
	}
	if got.SummaryText != "must survive the failed update" {
		t.Fatalf("expected the cached summary to be unchanged, got %q", got.SummaryText)
	}
	revision, err := summaryRepo.GetRevision(ctx, rm.ID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if revision != 0 {
		t.Fatalf("expected the invalidation revision to remain 0, got %d", revision)
	}
}
