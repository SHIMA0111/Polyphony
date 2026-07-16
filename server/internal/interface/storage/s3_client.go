// Package storage implements outbound storage adapters, including the
// aws-sdk-go-v2-backed S3Storage implementation of domain/storage.ObjectStorage.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage implements the domain/storage.ObjectStorage port using the
// aws-sdk-go-v2 S3 presign client. It never uploads or downloads object
// bytes itself; it only generates presigned URLs that let a caller (e.g. a
// browser) exchange bytes directly with the configured S3-compatible
// endpoint (AWS S3 or MinIO).
type S3Storage struct {
	presignClient *s3.PresignClient
	bucket        string
}

// NewS3Storage creates a new S3Storage configured with static credentials
// and pointed at the given endpoint. pathStyle should be true for
// S3-compatible stores that do not support virtual-hosted-style bucket
// addressing (e.g. MinIO); it maps to s3.Options.UsePathStyle. endpoint must
// be reachable by whichever caller will actually use the resulting presigned
// URL (see package-level docs in domain/storage and docs/tasks/step12.md for
// why this differs from the endpoint the API server itself would use).
func NewS3Storage(endpoint, region, bucket, accessKey, secretKey string, pathStyle bool) *S3Storage {
	client := s3.New(s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: pathStyle,
	})

	return &S3Storage{
		presignClient: s3.NewPresignClient(client),
		bucket:        bucket,
	}
}

// PresignUpload returns a presigned PUT URL for the given object key,
// content type, and exact size in bytes, valid for the given expiry.
//
// Setting ContentLength on the presigned request binds the declared size
// into the request's SigV4 signature: the object store rejects the upload
// if the actual request's Content-Length header does not match sizeBytes
// exactly, so this is an enforced upper (and lower) bound, not merely
// advisory.
func (s *S3Storage) PresignUpload(ctx context.Context, key, contentType string, sizeBytes int64, expires time.Duration) (string, error) {
	req, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(sizeBytes),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presign upload for key %q: %w", key, err)
	}
	return req.URL, nil
}

// PresignView returns a presigned GET URL for the given object key, valid
// for the given expiry.
func (s *S3Storage) PresignView(ctx context.Context, key string, expires time.Duration) (string, error) {
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presign view for key %q: %w", key, err)
	}
	return req.URL, nil
}
