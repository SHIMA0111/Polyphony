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

// TestRequestUpload_NonPositiveSize proves that a zero or negative declared
// size is rejected the same way an oversized one is, rather than being
// passed through to a presigned upload with a nonsensical content length.
func TestRequestUpload_NonPositiveSize(t *testing.T) {
	uc, _, roomRepo, _, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	for _, sizeBytes := range []int64{0, -1} {
		_, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", sizeBytes)
		if !errors.Is(err, domain.ErrAttachmentTooLarge) {
			t.Fatalf("sizeBytes=%d: expected ErrAttachmentTooLarge, got %v", sizeBytes, err)
		}
	}
}

// TestRequestUpload_BindsContentLength proves that RequestUpload passes the
// declared size through to storage.ObjectStorage.PresignUpload's
// contentLength parameter, so the presigned URL binds (and thereby enforces)
// the same size the caller declared.
func TestRequestUpload_BindsContentLength(t *testing.T) {
	uc, _, roomRepo, _, objStorage := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	ctx := context.Background()

	var gotContentLength int64
	objStorage.PresignUploadFunc = func(_ context.Context, key, _ string, contentLength int64, _ time.Duration) (string, error) {
		gotContentLength = contentLength
		return "https://mock-upload/" + key, nil
	}

	if _, err := uc.RequestUpload(ctx, "user-1", "room-1", "image/png", 4096); err != nil {
		t.Fatalf("RequestUpload failed: %v", err)
	}
	if gotContentLength != 4096 {
		t.Fatalf("expected PresignUpload to be called with contentLength 4096, got %d", gotContentLength)
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

// TestAttachToMessage_WrongRoom proves that an attachment uploaded into one
// room cannot be linked to a message in a different room, even by that
// message's own sender who is a member of both rooms — the security
// boundary that motivated adding Attachment.RoomID.
func TestAttachToMessage_WrongRoom(t *testing.T) {
	uc, attachmentRepo, roomRepo, msgRepo, _ := newTestUsecase()
	roomRepo.SeedMember("room-1", "user-1", "member")
	roomRepo.SeedMember("room-2", "user-1", "member")
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-2", SenderID: &senderID},
	}
	// Attachment was uploaded into room-1, not room-2.
	if err := attachmentRepo.Create(ctx, mustAttachmentInRoom("att-1", "room-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	_, err := uc.AttachToMessage(ctx, "user-1", "room-2", "msg-1", "att-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-room attachment, got %v", err)
	}
}

// mustAttachment builds a minimal, unlinked attachment fixture in room-1 for
// directly seeding a mocks.AttachmentRepo via Create, bypassing
// RequestUpload.
func mustAttachment(id string) *domainattachment.Attachment {
	return mustAttachmentInRoom(id, "room-1")
}

// mustAttachmentInRoom is mustAttachment with an explicit RoomID, for tests
// that need to seed an attachment belonging to a room other than "room-1".
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
