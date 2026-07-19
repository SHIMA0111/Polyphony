// Package attachment implements the business logic for uploading, linking,
// and viewing message attachments. It sits between the HTTP handlers
// (server/internal/interface/handler) and the attachment repository /
// object storage ports (server/internal/domain/attachment,
// server/internal/domain/storage).
package attachment

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/storage"
)

// MaxAttachmentSizeBytes is the maximum declared size, in bytes, accepted
// for an attachment upload (10 MiB).
const MaxAttachmentSizeBytes = 10 * 1024 * 1024

// uploadURLExpiry is how long a presigned upload URL remains valid.
const uploadURLExpiry = 15 * time.Minute

// viewURLExpiry is how long a presigned view URL remains valid.
const viewURLExpiry = time.Hour

// allowedMimeTypes is the allow-list of MIME types accepted for upload.
// Only image types are supported in this step; see phases.md Phase 12 and
// docs/tasks/step12.md's Out of scope section for the rationale.
var allowedMimeTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// UploadTicket is returned by RequestUpload: it carries everything the
// caller needs to upload bytes directly to the object store and later
// reference the resulting attachment.
type UploadTicket struct {
	AttachmentID string
	S3Key        string
	UploadURL    string
	ExpiresAt    time.Time
}

// AttachmentWithURL pairs an Attachment with a freshly-presigned view URL,
// as returned by ListAttachments.
type AttachmentWithURL struct {
	Attachment *domainattachment.Attachment
	ViewURL    string
}

// AttachmentUsecase provides attachment-related business logic: requesting
// presigned upload URLs, linking uploaded objects to messages, and listing a
// message's attachments with fresh presigned view URLs.
type AttachmentUsecase struct {
	attachmentRepo domainattachment.AttachmentRepository
	roomRepo       room.RoomRepository
	msgRepo        domainmessage.MessageRepository
	storage        storage.ObjectStorage
}

// NewAttachmentUsecase creates a new AttachmentUsecase.
func NewAttachmentUsecase(
	attachmentRepo domainattachment.AttachmentRepository,
	roomRepo room.RoomRepository,
	msgRepo domainmessage.MessageRepository,
	objStorage storage.ObjectStorage,
) *AttachmentUsecase {
	return &AttachmentUsecase{
		attachmentRepo: attachmentRepo,
		roomRepo:       roomRepo,
		msgRepo:        msgRepo,
		storage:        objStorage,
	}
}

// RequestUpload verifies the caller is a member of roomID, validates
// mimeType against the supported allow-list and sizeBytes (must be positive
// and no larger than MaxAttachmentSizeBytes), persists a new Attachment row
// scoped to roomID (not yet linked to any message), and returns an
// UploadTicket containing a presigned PUT URL valid for 15 minutes. The
// presigned URL itself binds sizeBytes as a required Content-Length (see
// domain/storage.ObjectStorage.PresignUpload), so the enforcement holds even
// if the client bypasses this API and uploads directly.
//
// It returns domain.ErrForbidden if the caller is not a room member,
// domain.ErrUnsupportedMimeType if mimeType is not in the allow-list,
// domain.ErrInvalidAttachmentSize if sizeBytes is not positive, and
// domain.ErrAttachmentTooLarge if sizeBytes exceeds MaxAttachmentSizeBytes.
func (u *AttachmentUsecase) RequestUpload(ctx context.Context, userID, roomID, mimeType string, sizeBytes int64) (*UploadTicket, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	if !allowedMimeTypes[mimeType] {
		return nil, domain.ErrUnsupportedMimeType
	}
	if sizeBytes <= 0 {
		return nil, domain.ErrInvalidAttachmentSize
	}
	if sizeBytes > MaxAttachmentSizeBytes {
		return nil, domain.ErrAttachmentTooLarge
	}

	attachmentID := uuid.New().String()
	s3Key := "attachments/" + roomID + "/" + uuid.New().String()

	a := &domainattachment.Attachment{
		ID:        attachmentID,
		RoomID:    roomID,
		MessageID: nil,
		S3Key:     s3Key,
		MimeType:  mimeType,
		SizeBytes: sizeBytes,
		CreatedAt: time.Now(),
	}
	if err := u.attachmentRepo.Create(ctx, a); err != nil {
		return nil, err
	}

	uploadURL, err := u.storage.PresignUpload(ctx, s3Key, mimeType, sizeBytes, uploadURLExpiry)
	if err != nil {
		slog.Default().Error("presign upload failed", "error", err, "s3_key", s3Key)
		return nil, err
	}

	return &UploadTicket{
		AttachmentID: attachmentID,
		S3Key:        s3Key,
		UploadURL:    uploadURL,
		ExpiresAt:    time.Now().Add(uploadURLExpiry),
	}, nil
}

// AttachToMessage links a previously-uploaded attachment to an existing
// message. Only the message's own sender may attach to it, and the
// attachment must have been uploaded into the same room as the message
// (enforced atomically by attachmentRepo.AttachToMessage's UPDATE
// predicate, closing the race between this check and the write). It returns
// domain.ErrForbidden if the caller is not a room member or is not the
// message's sender, domain.ErrNotFound if the message does not belong to
// roomID or the attachment does not exist/belongs to a different room, and
// domain.ErrAttachmentAlreadyLinked if the attachment is already linked.
func (u *AttachmentUsecase) AttachToMessage(ctx context.Context, userID, roomID, messageID, attachmentID string) (*domainattachment.Attachment, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	if msg.RoomID != roomID {
		return nil, domain.ErrNotFound
	}
	if msg.SenderID == nil || *msg.SenderID != userID {
		return nil, domain.ErrForbidden
	}

	return u.attachmentRepo.AttachToMessage(ctx, attachmentID, messageID, roomID)
}

// ListAttachments verifies the caller is a room member and that messageID
// belongs to roomID, then returns every attachment linked to that message
// paired with a freshly-presigned view URL valid for 1 hour.
//
// It returns domain.ErrForbidden if the caller is not a room member and
// domain.ErrNotFound if the message does not belong to roomID.
func (u *AttachmentUsecase) ListAttachments(ctx context.Context, userID, roomID, messageID string) ([]AttachmentWithURL, error) {
	if err := u.checkMembership(ctx, roomID, userID); err != nil {
		return nil, err
	}

	msg, err := u.msgRepo.GetByID(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	if msg.RoomID != roomID {
		return nil, domain.ErrNotFound
	}

	attachments, err := u.attachmentRepo.ListByMessageID(ctx, messageID)
	if err != nil {
		return nil, err
	}

	result := make([]AttachmentWithURL, len(attachments))
	for i, a := range attachments {
		viewURL, err := u.storage.PresignView(ctx, a.S3Key, viewURLExpiry)
		if err != nil {
			slog.Default().Error("presign view failed", "error", err, "s3_key", a.S3Key)
			return nil, err
		}
		result[i] = AttachmentWithURL{Attachment: a, ViewURL: viewURL}
	}
	return result, nil
}

// checkMembership verifies that userID is a member of roomID, mapping a
// domain.ErrNotFound membership lookup to domain.ErrForbidden so callers
// cannot distinguish "room does not exist" from "not a member" (mirroring
// message.MessageUsecase.checkMembership).
func (u *AttachmentUsecase) checkMembership(ctx context.Context, roomID, userID string) error {
	_, err := u.roomRepo.GetMember(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrForbidden
		}
		return err
	}
	return nil
}
