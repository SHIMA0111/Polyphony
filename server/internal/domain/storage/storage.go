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
	// object with the given key, content type, and exact size in bytes. The
	// URL expires after the given duration.
	//
	// sizeBytes is bound into the presigned request's signature (as a
	// required Content-Length), not merely advisory: the implementation
	// must reject an upload whose actual body size does not match
	// sizeBytes, so a caller cannot request a small presigned upload and
	// then stream a larger object through it.
	//
	// If err is non-nil, the returned url is empty/invalid and must not be
	// used.
	PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, expires time.Duration) (url string, err error)

	// PresignView returns a presigned URL that can be used to GET the
	// object with the given key. The URL expires after the given duration.
	//
	// If err is non-nil, the returned url is empty/invalid and must not be
	// used.
	PresignView(ctx context.Context, key string, expires time.Duration) (url string, err error)
}
