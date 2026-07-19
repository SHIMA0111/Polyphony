package mocks

import (
	"context"
	"fmt"
	"time"
)

// ObjectStorage is a configurable fake implementing storage.ObjectStorage.
//
// By default, PresignUpload and PresignView return canned deterministic URLs
// derived from the given key, prefixed "https://mock-upload/" and
// "https://mock-view/" respectively. Setting ShouldErr makes both methods
// return an error instead. For full control, set PresignUploadFunc /
// PresignViewFunc, which take priority over ShouldErr.
//
// The zero value (mocks.ObjectStorage{}) is ready to use.
type ObjectStorage struct {
	// ShouldErr, if true, makes both methods return an error instead of a
	// canned URL.
	ShouldErr bool

	// PresignUploadFunc, if set, overrides PresignUpload entirely.
	PresignUploadFunc func(ctx context.Context, key, contentType string, sizeBytes int64, expires time.Duration) (string, error)
	// PresignViewFunc, if set, overrides PresignView entirely.
	PresignViewFunc func(ctx context.Context, key string, expires time.Duration) (string, error)
}

// PresignUpload returns a presigned upload URL. See the ObjectStorage doc
// comment for how ShouldErr and PresignUploadFunc interact.
func (s *ObjectStorage) PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, expires time.Duration) (string, error) {
	if s.PresignUploadFunc != nil {
		return s.PresignUploadFunc(ctx, key, contentType, sizeBytes, expires)
	}
	if s.ShouldErr {
		return "", fmt.Errorf("mock object storage: presign upload error")
	}
	return "https://mock-upload/" + key, nil
}

// PresignView returns a presigned view URL. See the ObjectStorage doc
// comment for how ShouldErr and PresignViewFunc interact.
func (s *ObjectStorage) PresignView(ctx context.Context, key string, expires time.Duration) (string, error) {
	if s.PresignViewFunc != nil {
		return s.PresignViewFunc(ctx, key, expires)
	}
	if s.ShouldErr {
		return "", fmt.Errorf("mock object storage: presign view error")
	}
	return "https://mock-view/" + key, nil
}
