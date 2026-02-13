package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Compile-time check: OSSBackend implements StorageBackend.
var _ StorageBackend = (*OSSBackend)(nil)

// OSSBackend wraps the Alibaba Cloud OSS SDK.
type OSSBackend struct {
	bucket *oss.Bucket
}

// NewOSSBackend creates a new Alibaba Cloud OSS backend.
func NewOSSBackend(endpoint, keyID, keySecret, bucketName string) (*OSSBackend, error) {
	client, err := oss.New(endpoint, keyID, keySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	log.Printf("OSS backend initialized: endpoint=%s, bucket=%s", endpoint, bucketName)
	return &OSSBackend{bucket: bucket}, nil
}

// Upload uploads an object to OSS. size is ignored (OSS SDK reads until EOF).
func (b *OSSBackend) Upload(_ context.Context, key string, reader io.Reader, _ int64, contentType string) error {
	var opts []oss.Option
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}

	if err := b.bucket.PutObject(key, reader, opts...); err != nil {
		return fmt.Errorf("failed to upload %s: %w", key, err)
	}
	return nil
}

// Download downloads an object from OSS.
func (b *OSSBackend) Download(_ context.Context, key string) (io.ReadCloser, error) {
	body, err := b.bucket.GetObject(key)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", key, err)
	}
	return body, nil
}

// Delete deletes a single object from OSS.
func (b *OSSBackend) Delete(_ context.Context, key string) error {
	if err := b.bucket.DeleteObject(key); err != nil {
		return fmt.Errorf("failed to delete %s: %w", key, err)
	}
	return nil
}

// DeletePrefix deletes all objects under a given prefix.
func (b *OSSBackend) DeletePrefix(_ context.Context, prefix string) error {
	marker := ""
	for {
		result, err := b.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(1000))
		if err != nil {
			return fmt.Errorf("failed to list objects for deletion: %w", err)
		}

		var keys []string
		for _, obj := range result.Objects {
			keys = append(keys, obj.Key)
		}
		if len(keys) > 0 {
			_, err = b.bucket.DeleteObjects(keys)
			if err != nil {
				return fmt.Errorf("failed to batch delete objects: %w", err)
			}
		}

		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
	}
	return nil
}

// List lists objects under a prefix. If delimiter is non-empty, it simulates directories.
func (b *OSSBackend) List(_ context.Context, prefix, delimiter string, maxKeys int) ([]ObjectInfo, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	var opts []oss.Option
	opts = append(opts, oss.Prefix(prefix), oss.MaxKeys(maxKeys))
	if delimiter != "" {
		opts = append(opts, oss.Delimiter(delimiter))
	}

	result, err := b.bucket.ListObjects(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var items []ObjectInfo

	// Common prefixes (directories)
	for _, cp := range result.CommonPrefixes {
		items = append(items, ObjectInfo{
			Key:         cp,
			IsDirectory: true,
		})
	}

	// Objects (files)
	for _, obj := range result.Objects {
		// Skip the prefix itself if it appears as an object
		if obj.Key == prefix {
			continue
		}
		items = append(items, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			IsDirectory:  false,
		})
	}

	return items, nil
}

// ListAll lists all objects under a prefix (no pagination limit).
func (b *OSSBackend) ListAll(_ context.Context, prefix string) ([]ObjectInfo, error) {
	var items []ObjectInfo
	marker := ""
	for {
		result, err := b.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(1000))
		if err != nil {
			return nil, fmt.Errorf("failed to list all objects: %w", err)
		}

		for _, obj := range result.Objects {
			if obj.Key == prefix {
				continue
			}
			items = append(items, ObjectInfo{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				IsDirectory:  false,
			})
		}

		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
	}
	return items, nil
}

// GeneratePresignedURL generates a pre-signed download URL.
func (b *OSSBackend) GeneratePresignedURL(_ context.Context, key string, expiry time.Duration) (string, error) {
	url, err := b.bucket.SignURL(key, oss.HTTPGet, int64(expiry.Seconds()))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url, nil
}

// Copy copies an object within the bucket.
func (b *OSSBackend) Copy(_ context.Context, srcKey, destKey string) error {
	_, err := b.bucket.CopyObject(srcKey, destKey)
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", srcKey, destKey, err)
	}
	return nil
}

// GetObjectSize returns the size of an object.
func (b *OSSBackend) GetObjectSize(_ context.Context, key string) (int64, error) {
	props, err := b.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		return 0, fmt.Errorf("failed to get object meta: %w", err)
	}
	// Content-Length header
	sizeStr := props.Get("Content-Length")
	var size int64
	fmt.Sscanf(sizeStr, "%d", &size)
	return size, nil
}
