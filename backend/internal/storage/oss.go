package storage

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSClient wraps the Alibaba Cloud OSS SDK.
type OSSClient struct {
	bucket *oss.Bucket
}

// NewOSSClient creates a new OSS client.
func NewOSSClient(endpoint, keyID, keySecret, bucketName string) (*OSSClient, error) {
	client, err := oss.New(endpoint, keyID, keySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	log.Printf("OSS client initialized: endpoint=%s, bucket=%s", endpoint, bucketName)
	return &OSSClient{bucket: bucket}, nil
}

// Upload uploads an object to OSS.
func (c *OSSClient) Upload(key string, reader io.Reader, contentType string) error {
	var opts []oss.Option
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}

	if err := c.bucket.PutObject(key, reader, opts...); err != nil {
		return fmt.Errorf("failed to upload %s: %w", key, err)
	}
	return nil
}

// Download downloads an object from OSS.
func (c *OSSClient) Download(key string) (io.ReadCloser, error) {
	body, err := c.bucket.GetObject(key)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", key, err)
	}
	return body, nil
}

// Delete deletes a single object from OSS.
func (c *OSSClient) Delete(key string) error {
	if err := c.bucket.DeleteObject(key); err != nil {
		return fmt.Errorf("failed to delete %s: %w", key, err)
	}
	return nil
}

// DeletePrefix deletes all objects under a given prefix.
func (c *OSSClient) DeletePrefix(prefix string) error {
	marker := ""
	for {
		result, err := c.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(1000))
		if err != nil {
			return fmt.Errorf("failed to list objects for deletion: %w", err)
		}

		var keys []string
		for _, obj := range result.Objects {
			keys = append(keys, obj.Key)
		}
		if len(keys) > 0 {
			_, err = c.bucket.DeleteObjects(keys)
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

// ObjectInfo represents a listed OSS object.
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	IsDirectory  bool      `json:"is_directory"`
}

// List lists objects under a prefix. If delimiter is non-empty, it simulates directories.
func (c *OSSClient) List(prefix, delimiter string, maxKeys int) ([]ObjectInfo, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	var opts []oss.Option
	opts = append(opts, oss.Prefix(prefix), oss.MaxKeys(maxKeys))
	if delimiter != "" {
		opts = append(opts, oss.Delimiter(delimiter))
	}

	result, err := c.bucket.ListObjects(opts...)
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
func (c *OSSClient) ListAll(prefix string) ([]ObjectInfo, error) {
	var items []ObjectInfo
	marker := ""
	for {
		result, err := c.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(1000))
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
func (c *OSSClient) GeneratePresignedURL(key string, expiry time.Duration) (string, error) {
	url, err := c.bucket.SignURL(key, oss.HTTPGet, int64(expiry.Seconds()))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url, nil
}

// Copy copies an object within the bucket.
func (c *OSSClient) Copy(srcKey, destKey string) error {
	_, err := c.bucket.CopyObject(srcKey, destKey)
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", srcKey, destKey, err)
	}
	return nil
}

// GetObjectSize returns the size of an object.
func (c *OSSClient) GetObjectSize(key string) (int64, error) {
	props, err := c.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		return 0, fmt.Errorf("failed to get object meta: %w", err)
	}
	// Content-Length header
	sizeStr := props.Get("Content-Length")
	var size int64
	fmt.Sscanf(sizeStr, "%d", &size)
	return size, nil
}
