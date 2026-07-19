package attachment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func newTestUsecase() (*AttachmentUsecase, *mocks.AttachmentRepo, *mocks.RoomRepo, *mocks.MessageRepo, *mocks.ObjectStorage) {
	attachmentRepo := &mocks.AttachmentRepo{}
	roomRepo := &mocks.RoomRepo{}
	msgRepo := &mocks.MessageRepo{}
	objStorage := &mocks.ObjectStorage{}
	uc := NewAttachmentUsecase(attachmentRepo, roomRepo, msgRepo, objStorage)
	return uc, attachmentRepo, roomRepo, msgRepo, objStorage
}

func TestRequestUpload_HappyPath(t *testing.T) {
	uc, attachmentRepo, roomRepo, _, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	ticket, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", 1024)
	if err != nil {
		t.Fatalf("RequestUpload failed: %v", err)
	}
	if ticket.AttachmentID == "" {
		t.Fatal("expected non-empty AttachmentID")
	}
	if ticket.UploadURL == "" {
		t.Fatal("expected non-empty UploadURL")
	}
	if ticket.S3Key == "" {
		t.Fatal("expected non-empty S3Key")
	}
	if !ticket.ExpiresAt.After(time.Now()) {
		t.Fatal("expected ExpiresAt in the future")
	}

	stored, err := attachmentRepo.GetByID(ctx, ticket.AttachmentID)
	if err != nil {
		t.Fatalf("expected attachment to be persisted, got error: %v", err)
	}
	if stored.MessageID != nil {
		t.Fatal("expected MessageID to be nil at upload-request time")
	}
}

func TestRequestUpload_NotMember(t *testing.T) {
	uc, _, _, _, _ := newTestUsecase()
	ctx := context.Background()

	_, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", 1024)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestRequestUpload_UnsupportedMimeType(t *testing.T) {
	uc, _, roomRepo, _, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	_, err := uc.RequestUpload(ctx, "user-1", "room-1", "application/pdf", 1024)
	if !errors.Is(err, domain.ErrUnsupportedMimeType) {
		t.Fatalf("expected ErrUnsupportedMimeType, got %v", err)
	}
}

func TestRequestUpload_TooLarge(t *testing.T) {
	uc, _, roomRepo, _, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	_, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", MaxAttachmentSizeBytes+1)
	if !errors.Is(err, domain.ErrAttachmentTooLarge) {
		t.Fatalf("expected ErrAttachmentTooLarge, got %v", err)
	}
}

// TestRequestUpload_InvalidSize proves that zero and negative declared
// sizes are rejected with domain.ErrInvalidAttachmentSize rather than being
// treated as valid (and, in the zero case, never accidentally passing the
// `sizeBytes > MaxAttachmentSizeBytes` check meant to catch oversized
// uploads).
func TestRequestUpload_InvalidSize(t *testing.T) {
	for _, sizeBytes := range []int64{0, -1} {
		uc, _, roomRepo, _, _ := newTestUsecase()
		roomRepo.SeedMember("room-1", "user-1", "member")
		ctx := context.Background()

		_, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", sizeBytes)
		if !errors.Is(err, domain.ErrInvalidAttachmentSize) {
			t.Fatalf("sizeBytes=%d: expected ErrInvalidAttachmentSize, got %v", sizeBytes, err)
		}
	}
}

func TestAttachToMessage_HappyPath(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, mustAttachment("att-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	a, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-1", "att-1")
	if err != nil {
		t.Fatalf("AttachToMessage failed: %v", err)
	}
	if a.MessageID == nil || *a.MessageID != "msg-1" {
		t.Fatalf("expected MessageID msg-1, got %v", a.MessageID)
	}
}

func TestAttachToMessage_NotSender(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	otherSender := "user-2"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &otherSender},
	}
	if err := attachmentRepo.Create(ctx, mustAttachment("att-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	_, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-1", "att-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// TestAttachToMessage_CrossRoom proves that an attachment uploaded into one
// room cannot be attached to a message in a different room, even if the
// caller is a member of both rooms and the sender of the message: this is
// the cross-room attach attempt that domain.ErrNotFound must reject,
// closing off using another room's attachment to leak its content into a
// message the attacker fully controls.
func TestAttachToMessage_CrossRoom(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-2", "user-1", "member")
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	// Attachment was uploaded into room-2, not room-1.
	if err := attachmentRepo.Create(ctx, mustAttachmentInRoom("att-1", "room-2")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	_, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-1", "att-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a cross-room attach attempt, got %v", err)
	}
}

func TestAttachToMessage_AlreadyLinked(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
		"msg-2": {ID: "msg-2", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, mustAttachment("att-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	if _, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-1", "att-1"); err != nil {
		t.Fatalf("first attach failed: %v", err)
	}

	_, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-2", "att-1")
	if !errors.Is(err, domain.ErrAttachmentAlreadyLinked) {
		t.Fatalf("expected ErrAttachmentAlreadyLinked, got %v", err)
	}
}

func TestListAttachments_WithViewURLs(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, mustAttachment("att-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}
	if _, err := uc.AttachToMessage(ctx, "user-1", "room-1", "msg-1", "att-1"); err != nil {
		t.Fatalf("attach failed: %v", err)
	}

	list, err := uc.ListAttachments(ctx, "user-1", "room-1", "msg-1")
	if err != nil {
		t.Fatalf("ListAttachments failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(list))
	}
	if list[0].ViewURL == "" {
		t.Fatal("expected non-empty ViewURL")
	}
}

// mustAttachment builds a minimal, unlinked attachment fixture scoped to
// "room-1" for directly seeding a mocks.AttachmentRepo via Create, bypassing
// RequestUpload.
func mustAttachment(id string) *domainattachment.Attachment {
	return mustAttachmentInRoom(id, "room-1")
}

// mustAttachmentInRoom is mustAttachment with an explicit RoomID, for tests
// exercising cross-room attach attempts.
func mustAttachmentInRoom(id, roomID string) *domainattachment.Attachment {
	return &domainattachment.Attachment{
		ID:        id,
		RoomID:    roomID,
		MessageID: nil,
		S3Key:     "attachments/" + roomID + "/" + id,
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}
}
