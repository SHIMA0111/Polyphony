//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// seedMessage creates a message in the given room, reserving its own
// sequence number, and returns it. It is a small shared setup helper for the
// attachment repository integration tests in this file.
func seedMessage(ctx context.Context, t *testing.T, msgRepo *MessageRepository, roomID, senderID string) *domainmessage.Message {
	t.Helper()

	seq, err := msgRepo.ReserveSequenceRange(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("reserve sequence: %v", err)
	}

	now := time.Now()
	msg := &domainmessage.Message{
		ID:         uuid.New().String(),
		RoomID:     roomID,
		SenderID:   &senderID,
		Content:    "hello",
		Type:       domainmessage.MessageTypeHuman,
		Status:     domainmessage.MessageStatusCompleted,
		Sequence:   seq,
		Visibility: domainmessage.MessageVisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := msgRepo.Create(ctx, msg); err != nil {
		t.Fatalf("create message: %v", err)
	}
	return msg
}

// TestAttachmentRepository_CreateAndGetByID proves that a freshly-created
// attachment (with a nil MessageID, as it would be at upload-request time)
// round-trips through Create/GetByID.
func TestAttachmentRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-create-owner")

	a := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		MessageID: nil,
		S3Key:     "attachments/" + rm.ID + "/" + uuid.New().String(),
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, a); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := attachmentRepo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.MessageID != nil {
		t.Fatalf("expected nil MessageID, got %v", *got.MessageID)
	}
	if got.RoomID != a.RoomID || got.S3Key != a.S3Key || got.MimeType != a.MimeType || got.SizeBytes != a.SizeBytes {
		t.Fatalf("round-tripped attachment mismatch: got %+v, want %+v", got, a)
	}
}

// TestAttachmentRepository_GetByIDNotFound proves that GetByID maps a
// missing row to domain.ErrNotFound.
func TestAttachmentRepository_GetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	attachmentRepo := NewAttachmentRepository(pool)

	_, err := attachmentRepo.GetByID(ctx, uuid.New().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

// TestAttachmentRepository_AttachToMessage proves the happy path: an
// unlinked attachment can be attached to a message, and GetByID afterward
// reflects the link.
func TestAttachmentRepository_AttachToMessage(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-link-owner")
	msg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)

	a := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/" + uuid.New().String(),
		MimeType:  "image/jpeg",
		SizeBytes: 2048,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, a); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	linked, err := attachmentRepo.AttachToMessage(ctx, a.ID, msg.ID, rm.ID)
	if err != nil {
		t.Fatalf("AttachToMessage failed: %v", err)
	}
	if linked.MessageID == nil || *linked.MessageID != msg.ID {
		t.Fatalf("expected MessageID %s, got %v", msg.ID, linked.MessageID)
	}

	got, err := attachmentRepo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.MessageID == nil || *got.MessageID != msg.ID {
		t.Fatalf("expected persisted MessageID %s, got %v", msg.ID, got.MessageID)
	}
}

// TestAttachmentRepository_AttachToMessageAlreadyLinked proves the conflict
// path: attaching an already-linked attachment again (even to the same
// message) fails with domain.ErrAttachmentAlreadyLinked, and the original
// link is left untouched.
func TestAttachmentRepository_AttachToMessageAlreadyLinked(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-conflict-owner")
	firstMsg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)
	secondMsg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)

	a := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/" + uuid.New().String(),
		MimeType:  "image/gif",
		SizeBytes: 512,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, a); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, a.ID, firstMsg.ID, rm.ID); err != nil {
		t.Fatalf("first AttachToMessage failed: %v", err)
	}

	_, err := attachmentRepo.AttachToMessage(ctx, a.ID, secondMsg.ID, rm.ID)
	if !errors.Is(err, domain.ErrAttachmentAlreadyLinked) {
		t.Fatalf("expected domain.ErrAttachmentAlreadyLinked, got %v", err)
	}

	got, err := attachmentRepo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.MessageID == nil || *got.MessageID != firstMsg.ID {
		t.Fatalf("expected link to remain on first message %s, got %v", firstMsg.ID, got.MessageID)
	}
}

// TestAttachmentRepository_AttachToMessageNotFound proves that attaching a
// non-existent attachment ID returns domain.ErrNotFound rather than
// domain.ErrAttachmentAlreadyLinked.
func TestAttachmentRepository_AttachToMessageNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-missing-owner")
	msg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)

	_, err := attachmentRepo.AttachToMessage(ctx, uuid.New().String(), msg.ID, rm.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

// TestAttachmentRepository_AttachToMessageCrossRoom proves that attaching an
// attachment to a message in a *different* room than the one the attachment
// was uploaded into returns domain.ErrNotFound (indistinguishable from a
// missing attachment ID) rather than succeeding, and leaves the attachment
// unlinked.
func TestAttachmentRepository_AttachToMessageCrossRoom(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	roomA := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-cross-room-a")
	roomB := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-cross-room-b")
	msgInRoomA := seedMessage(ctx, t, msgRepo, roomA.ID, roomA.OwnerID)

	// Attachment uploaded into roomB, not roomA.
	a := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    roomB.ID,
		S3Key:     "attachments/" + roomB.ID + "/" + uuid.New().String(),
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, a); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err := attachmentRepo.AttachToMessage(ctx, a.ID, msgInRoomA.ID, roomA.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for a cross-room attach attempt, got %v", err)
	}

	got, err := attachmentRepo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.MessageID != nil {
		t.Fatalf("expected attachment to remain unlinked after cross-room attach attempt, got %v", *got.MessageID)
	}
}

// TestAttachmentRepository_ListByMessageID proves that ListByMessageID
// returns only attachments linked to the given message, ordered by creation
// time ascending, and excludes unlinked or differently-linked attachments.
func TestAttachmentRepository_ListByMessageID(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	attachmentRepo := NewAttachmentRepository(pool)

	rm := seedUserAndRoom(ctx, t, userRepo, roomRepo, "attach-list-owner")
	targetMsg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)
	otherMsg := seedMessage(ctx, t, msgRepo, rm.ID, rm.OwnerID)

	// Unlinked attachment: must not appear in the list.
	unlinked := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/unlinked",
		MimeType:  "image/png",
		SizeBytes: 1,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, unlinked); err != nil {
		t.Fatalf("Create unlinked failed: %v", err)
	}

	// Attachment linked to a different message: must not appear in the list.
	otherLinked := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/other",
		MimeType:  "image/png",
		SizeBytes: 1,
		CreatedAt: time.Now(),
	}
	if err := attachmentRepo.Create(ctx, otherLinked); err != nil {
		t.Fatalf("Create otherLinked failed: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, otherLinked.ID, otherMsg.ID, rm.ID); err != nil {
		t.Fatalf("AttachToMessage otherLinked failed: %v", err)
	}

	// Two attachments linked to the target message, created in order. Explicit,
	// distinct timestamps (rather than back-to-back time.Now() calls) so the
	// ordering assertion below never depends on wall-clock resolution.
	base := time.Now().UTC().Truncate(time.Microsecond)
	first := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/first",
		MimeType:  "image/png",
		SizeBytes: 1,
		CreatedAt: base,
	}
	if err := attachmentRepo.Create(ctx, first); err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, first.ID, targetMsg.ID, rm.ID); err != nil {
		t.Fatalf("AttachToMessage first failed: %v", err)
	}

	second := &domainattachment.Attachment{
		ID:        uuid.New().String(),
		RoomID:    rm.ID,
		S3Key:     "attachments/" + rm.ID + "/second",
		MimeType:  "image/png",
		SizeBytes: 1,
		CreatedAt: base.Add(time.Millisecond),
	}
	if err := attachmentRepo.Create(ctx, second); err != nil {
		t.Fatalf("Create second failed: %v", err)
	}
	if _, err := attachmentRepo.AttachToMessage(ctx, second.ID, targetMsg.ID, rm.ID); err != nil {
		t.Fatalf("AttachToMessage second failed: %v", err)
	}

	list, err := attachmentRepo.ListByMessageID(ctx, targetMsg.ID)
	if err != nil {
		t.Fatalf("ListByMessageID failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(list))
	}
	if list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("expected order [first, second], got [%s, %s]", list[0].ID, list[1].ID)
	}
}
