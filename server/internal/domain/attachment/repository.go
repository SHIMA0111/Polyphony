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
	// message, provided the attachment's RoomID matches roomID. It returns
	// domain.ErrNotFound if the attachment does not exist or belongs to a
	// different room (the two are deliberately indistinguishable to
	// callers, so a caller cannot use this to probe for the existence of
	// attachments in rooms it has no access to), and
	// domain.ErrAttachmentAlreadyLinked if the attachment is already linked
	// to a (possibly different) message.
	AttachToMessage(ctx context.Context, attachmentID, messageID, roomID string) (*Attachment, error)

	// ListByMessageID returns every attachment linked to the given message,
	// ordered by creation time ascending.
	ListByMessageID(ctx context.Context, messageID string) ([]*Attachment, error)

	// ListByMessageIDs is ListByMessageID's batch counterpart: it returns
	// every attachment linked to any of messageIDs in a single query,
	// grouped by message ID, with each group's slice ordered by creation
	// time ascending (matching ListByMessageID's per-message order). A
	// message ID with no attachments is simply absent from the returned map
	// rather than mapped to an empty/nil slice. Callers with N messages to
	// enrich should prefer this over N calls to ListByMessageID -- see
	// usecase/message.MessageUsecase.enrichWithAttachments, the motivating
	// caller, which previously issued one ListByMessageID call per message
	// in a context bucket.
	ListByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]*Attachment, error)
}
