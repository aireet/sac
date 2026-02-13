package storage

import (
	"context"
	"io"
	"time"
)

// StorageType identifies the object-storage backend in use.
type StorageType string

const (
	TypeOSS      StorageType = "oss"      // Alibaba Cloud OSS
	TypeS3       StorageType = "s3"       // AWS S3
	TypeS3Compat StorageType = "s3compat" // S3-compatible (MinIO, RustFS, Cloudflare R2, etc.)
)

// ObjectInfo represents a listed storage object.
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	IsDirectory  bool      `json:"is_directory"`
}

// StorageBackend is the pluggable interface that every object-storage
// implementation must satisfy.
type StorageBackend interface {
	// Upload stores an object. size may be -1 if unknown.
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error

	// Download retrieves an object. Caller must close the returned reader.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes a single object.
	Delete(ctx context.Context, key string) error

	// DeletePrefix removes all objects whose key starts with prefix.
	DeletePrefix(ctx context.Context, prefix string) error

	// List lists objects under prefix. If delimiter is non-empty it
	// simulates directory listing. maxKeys <= 0 means default (1000).
	List(ctx context.Context, prefix, delimiter string, maxKeys int) ([]ObjectInfo, error)

	// ListAll lists every object under prefix (handles pagination).
	ListAll(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// GeneratePresignedURL returns a time-limited download URL.
	GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// Copy duplicates an object within the same bucket.
	Copy(ctx context.Context, srcKey, destKey string) error

	// GetObjectSize returns the size in bytes of the stored object.
	GetObjectSize(ctx context.Context, key string) (int64, error)
}
