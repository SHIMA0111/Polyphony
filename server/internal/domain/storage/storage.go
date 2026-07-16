// Package storage defines the ObjectStorage port: the swap point between the
// domain/usecase layers and whatever S3-compatible object store backs
// attachment uploads/downloads (see interface/storage.S3Storage for the real
// aws-sdk-go-v2 adapter). This package must not import the AWS SDK or any
// other infrastructure package, matching the domain/ai.LLMGateway pattern.
package storage

import (
	"context"
	"time"
)

// ObjectStorage generates presigned URLs for uploading to and viewing
// objects in an S3-compatible object store. Implementations never read or
// write object bytes themselves: a presigned URL lets the caller (e.g. a
// browser) exchange bytes directly with the object store.
type ObjectStorage interface {
	// PresignUpload returns a presigned URL that can be used to PUT an
	// object with the given key, content type, and exact content length.
	// Binding contentLength into the signed request means the underlying
	// store rejects any PUT whose actual Content-Length header does not
	// match — the enforcement point that makes the declared size checked at
	// request time (see usecase/attachment.MaxAttachmentSizeBytes) binding
	// on the upload itself, rather than just a client-supplied claim the
	// caller could then ignore when PUTting to the URL. The URL expires
	// after the given duration.
	PresignUpload(ctx context.Context, key, contentType string, contentLength int64, expires time.Duration) (url string, err error)

	// PresignView returns a presigned URL that can be used to GET the
	// object with the given key. The URL expires after the given duration.
	PresignView(ctx context.Context, key string, expires time.Duration) (url string, err error)
}
