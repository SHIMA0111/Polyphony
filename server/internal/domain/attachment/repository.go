package attachment

import "context"

// AttachmentRepository defines persistence operations for message attachments.
type AttachmentRepository interface {
	// Create persists a new attachment row. MessageID is expected to be nil
	// at this point; it is populated later via AttachToMessage.
	Create(ctx context.Context, a *Attachment) error

	// GetByID retrieves an attachment by ID. Returns domain.ErrNotFound if
	// not found.
	GetByID(ctx context.Context, id string) (*Attachment, error)

	// AttachToMessage links an existing, previously-uploaded attachment to a
	// message, atomically enforcing that the attachment's RoomID matches
	// roomID (the room the message itself belongs to, as already verified
	// by the caller). It returns domain.ErrNotFound if the attachment does
	// not exist or belongs to a different room — the two are
	// indistinguishable to the caller so a cross-room attach attempt cannot
	// be used to probe for an attachment ID's existence — and
	// domain.ErrAttachmentAlreadyLinked if the attachment is already linked
	// to a (possibly different) message.
	AttachToMessage(ctx context.Context, attachmentID, messageID, roomID string) (*Attachment, error)

	// ListByMessageID returns every attachment linked to the given message,
	// ordered by creation time ascending.
	ListByMessageID(ctx context.Context, messageID string) ([]*Attachment, error)
}
