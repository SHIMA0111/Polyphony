package mocks

import (
	"context"
	"sort"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
)

// AttachmentRepo is an in-memory, map-backed fake implementing
// attachment.AttachmentRepository. The zero value (mocks.AttachmentRepo{})
// is ready to use; the backing map is initialized lazily on first write.
//
// AttachmentRepo is safe for concurrent use.
type AttachmentRepo struct {
	mu          sync.Mutex
	Attachments map[string]*attachment.Attachment
}

func (r *AttachmentRepo) ensureInit() {
	if r.Attachments == nil {
		r.Attachments = make(map[string]*attachment.Attachment)
	}
}

// cloneAttachment returns a deep-enough copy of a: a struct copy plus a
// fresh *string for MessageID when non-nil. A plain struct copy (`cp := *a`)
// still leaves cp.MessageID pointing at the very same string as a.MessageID,
// since copying a struct copies its pointer fields by value, not what they
// point to — so a caller mutating *cp.MessageID (or the repo later
// re-deriving a pointer from the same address) would silently alias the
// stored attachment. cloneAttachment is used for every value stored into or
// read out of r.Attachments so no caller can ever observe or corrupt the
// repo's internal state through a shared MessageID pointer.
func cloneAttachment(a *attachment.Attachment) *attachment.Attachment {
	cp := *a
	if a.MessageID != nil {
		msgID := *a.MessageID
		cp.MessageID = &msgID
	}
	return &cp
}

// Create persists a new attachment row.
func (r *AttachmentRepo) Create(_ context.Context, a *attachment.Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	// Store a clone so later in-place mutation by callers (or by
	// AttachToMessage below) can't alias the caller's own struct.
	r.Attachments[a.ID] = cloneAttachment(a)
	return nil
}

// GetByID retrieves an attachment by ID. Returns domain.ErrNotFound if not
// present.
func (r *AttachmentRepo) GetByID(_ context.Context, id string) (*attachment.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.Attachments[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneAttachment(a), nil
}

// AttachToMessage links an existing attachment to a message, provided the
// attachment's RoomID matches roomID. Returns domain.ErrNotFound if the
// attachment does not exist or belongs to a different room, and
// domain.ErrAttachmentAlreadyLinked if it is already linked to a message.
// Mirrors the room-scoping enforced by the real
// postgres.AttachmentRepository's UPDATE predicate.
func (r *AttachmentRepo) AttachToMessage(_ context.Context, attachmentID, messageID, roomID string) (*attachment.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.Attachments[attachmentID]
	if !ok || a.RoomID != roomID {
		return nil, domain.ErrNotFound
	}
	if a.MessageID != nil {
		return nil, domain.ErrAttachmentAlreadyLinked
	}
	msgID := messageID
	a.MessageID = &msgID
	return cloneAttachment(a), nil
}

// ListByMessageID returns every attachment linked to the given message,
// ordered by creation time ascending.
func (r *AttachmentRepo) ListByMessageID(_ context.Context, messageID string) ([]*attachment.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*attachment.Attachment
	for _, a := range r.Attachments {
		if a.MessageID != nil && *a.MessageID == messageID {
			result = append(result, cloneAttachment(a))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}
