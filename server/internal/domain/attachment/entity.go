// Package attachment defines the message attachment entity and its
// repository port. An attachment is a reference to an object stored in an
// S3-compatible object store (see domain/storage.ObjectStorage), optionally
// linked to a message once the client confirms the upload succeeded.
package attachment

import "time"

// Attachment represents a single uploaded object (currently image-only; see
// phases.md Phase 12) that may or may not yet be linked to a message.
//
// An Attachment row is created at upload-request time, before the client has
// actually uploaded any bytes to the presigned URL, and before it is linked
// to a message. MessageID is nil until AttachmentRepository.AttachToMessage
// links it.
type Attachment struct {
	// ID is the attachment's unique identifier.
	ID string
	// MessageID is the ID of the message this attachment is linked to, or
	// nil if it has not yet been attached to any message.
	MessageID *string
	// S3Key is the object key under which the attachment's bytes are (or
	// will be) stored in the configured S3 bucket.
	S3Key string
	// MimeType is the attachment's declared content type (e.g. "image/png").
	MimeType string
	// SizeBytes is the attachment's declared size in bytes.
	SizeBytes int64
	// CreatedAt is when the attachment row was created.
	CreatedAt time.Time
}
