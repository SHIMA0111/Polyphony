package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
)

// AttachmentRepository implements the attachment.AttachmentRepository
// interface using PostgreSQL.
type AttachmentRepository struct {
	pool *pgxpool.Pool
}

// NewAttachmentRepository creates a new AttachmentRepository backed by the
// given connection pool.
func NewAttachmentRepository(pool *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{pool: pool}
}

const attachmentColumns = `id, room_id, message_id, s3_key, mime_type, size_bytes, created_at`

// scanAttachment scans an attachment row into an Attachment struct.
func scanAttachment(scanner interface{ Scan(dest ...any) error }) (*attachment.Attachment, error) {
	var a attachment.Attachment
	if err := scanner.Scan(&a.ID, &a.RoomID, &a.MessageID, &a.S3Key, &a.MimeType, &a.SizeBytes, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// Create persists a new attachment row. MessageID is expected to be nil at
// this point.
func (r *AttachmentRepository) Create(ctx context.Context, a *attachment.Attachment) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO message_attachments (id, room_id, message_id, s3_key, mime_type, size_bytes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.ID, a.RoomID, a.MessageID, a.S3Key, a.MimeType, a.SizeBytes, a.CreatedAt,
	)
	return err
}

// GetByID retrieves an attachment by its unique identifier. It returns
// domain.ErrNotFound if the attachment does not exist.
func (r *AttachmentRepository) GetByID(ctx context.Context, id string) (*attachment.Attachment, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+attachmentColumns+` FROM message_attachments WHERE id = $1`, id,
	)
	a, err := scanAttachment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

// AttachToMessage links an existing attachment to a message. The UPDATE is
// conditioned on message_id IS NULL AND room_id = roomID, so that an
// attachment can only ever be linked once (message_id IS NULL), and only to
// a message in the same room it was uploaded into (room_id = roomID) — this
// is the enforcement point for cross-room attachment reuse (see
// domain/attachment.Attachment.RoomID's doc comment). If the attachment does
// not exist, or exists but belongs to a different room, this returns
// domain.ErrNotFound (the two cases are deliberately indistinguishable); if
// it exists in this room but is already linked (to this message or any
// other) it returns domain.ErrAttachmentAlreadyLinked.
func (r *AttachmentRepository) AttachToMessage(ctx context.Context, attachmentID, messageID, roomID string) (*attachment.Attachment, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE message_attachments SET message_id = $1 WHERE id = $2 AND room_id = $3 AND message_id IS NULL`,
		messageID, attachmentID, roomID,
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		// Distinguish "does not exist"/"wrong room" from "already linked"
		// so callers can map each to the right HTTP status, without letting
		// a caller learn from the response whether the attachment ID exists
		// in a room it does not belong to.
		existing, err := r.GetByID(ctx, attachmentID)
		if err != nil {
			return nil, err
		}
		if existing.RoomID != roomID {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrAttachmentAlreadyLinked
	}
	return r.GetByID(ctx, attachmentID)
}

// ListByMessageID returns every attachment linked to the given message,
// ordered by creation time ascending.
func (r *AttachmentRepository) ListByMessageID(ctx context.Context, messageID string) ([]*attachment.Attachment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+attachmentColumns+` FROM message_attachments WHERE message_id = $1 ORDER BY created_at ASC`,
		messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []*attachment.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attachments, nil
}
