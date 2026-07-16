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

// Create persists a new attachment row.
func (r *AttachmentRepo) Create(_ context.Context, a *attachment.Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	// Store a copy so later in-place mutation by callers (or by
	// AttachToMessage below) can't alias the caller's own struct.
	cp := *a
	r.Attachments[a.ID] = &cp
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
	cp := *a
	return &cp, nil
}

// AttachToMessage links an existing attachment to a message. Returns
// domain.ErrNotFound if the attachment does not exist or belongs to a
// different room than roomID, and domain.ErrAttachmentAlreadyLinked if it is
// already linked to a message.
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
	cp := *a
	return &cp, nil
}

// ListByMessageID returns every attachment linked to the given message,
// ordered by creation time ascending.
func (r *AttachmentRepo) ListByMessageID(_ context.Context, messageID string) ([]*attachment.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []*attachment.Attachment
	for _, a := range r.Attachments {
		if a.MessageID != nil && *a.MessageID == messageID {
			cp := *a
			result = append(result, &cp)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}
