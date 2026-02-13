package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Compile-time check: S3CompatBackend implements StorageBackend.
var _ StorageBackend = (*S3CompatBackend)(nil)

// S3CompatConfig configures an S3-compatible backend (AWS S3, MinIO, RustFS, etc.).
type S3CompatConfig struct {
	Region          string
	Endpoint        string // custom endpoint URL (empty for real AWS)
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	UsePathStyle    bool // true for MinIO / RustFS
	ForceHTTP       bool // true to use http:// instead of https://
}

// S3CompatBackend wraps the AWS S3 SDK v2 and works with any S3-compatible service.
type S3CompatBackend struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// NewS3CompatBackend creates a StorageBackend backed by an S3-compatible service.
func NewS3CompatBackend(cfg S3CompatConfig) (*S3CompatBackend, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1" // default for MinIO / RustFS
	}

	opts := func(o *s3.Options) {
		o.Region = cfg.Region
		o.Credentials = credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, "",
		)
		o.UsePathStyle = cfg.UsePathStyle

		if cfg.Endpoint != "" {
			scheme := "https"
			if cfg.ForceHTTP {
				scheme = "http"
			}
			o.BaseEndpoint = aws.String(fmt.Sprintf("%s://%s", scheme, cfg.Endpoint))
		}
	}

	client := s3.New(s3.Options{}, opts)
	presigner := s3.NewPresignClient(client)

	log.Printf("S3-compat backend initialized: endpoint=%s, bucket=%s, pathStyle=%v",
		cfg.Endpoint, cfg.Bucket, cfg.UsePathStyle)

	return &S3CompatBackend{
		client:    client,
		presigner: presigner,
		bucket:    cfg.Bucket,
	}, nil
}

// Upload stores an object. size may be -1 if unknown.
func (b *S3CompatBackend) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   reader,
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	_, err := b.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload %s: %w", key, err)
	}
	return nil
}

// Download retrieves an object. Caller must close the returned reader.
func (b *S3CompatBackend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", key, err)
	}
	return output.Body, nil
}

// Delete removes a single object.
func (b *S3CompatBackend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete %s: %w", key, err)
	}
	return nil
}

// DeletePrefix removes all objects whose key starts with prefix.
func (b *S3CompatBackend) DeletePrefix(ctx context.Context, prefix string) error {
	var continuationToken *string
	for {
		listOut, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(b.bucket),
			Prefix:            aws.String(prefix),
			MaxKeys:           aws.Int32(1000),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return fmt.Errorf("failed to list objects for deletion: %w", err)
		}

		if len(listOut.Contents) == 0 {
			break
		}

		var objects []types.ObjectIdentifier
		for _, obj := range listOut.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}

		_, err = b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("failed to batch delete objects: %w", err)
		}

		if !aws.ToBool(listOut.IsTruncated) {
			break
		}
		continuationToken = listOut.NextContinuationToken
	}
	return nil
}

// List lists objects under prefix. If delimiter is non-empty it simulates directories.
func (b *S3CompatBackend) List(ctx context.Context, prefix, delimiter string, maxKeys int) ([]ObjectInfo, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(b.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(maxKeys)),
	}
	if delimiter != "" {
		input.Delimiter = aws.String(delimiter)
	}

	output, err := b.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var items []ObjectInfo

	// Common prefixes (directories)
	for _, cp := range output.CommonPrefixes {
		items = append(items, ObjectInfo{
			Key:         aws.ToString(cp.Prefix),
			IsDirectory: true,
		})
	}

	// Objects (files)
	for _, obj := range output.Contents {
		key := aws.ToString(obj.Key)
		if key == prefix {
			continue
		}
		items = append(items, ObjectInfo{
			Key:          key,
			Size:         aws.ToInt64(obj.Size),
			LastModified: aws.ToTime(obj.LastModified),
			IsDirectory:  false,
		})
	}

	return items, nil
}

// ListAll lists every object under prefix (handles pagination).
func (b *S3CompatBackend) ListAll(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var items []ObjectInfo
	var continuationToken *string

	for {
		output, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(b.bucket),
			Prefix:            aws.String(prefix),
			MaxKeys:           aws.Int32(1000),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list all objects: %w", err)
		}

		for _, obj := range output.Contents {
			key := aws.ToString(obj.Key)
			if key == prefix {
				continue
			}
			items = append(items, ObjectInfo{
				Key:          key,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				IsDirectory:  false,
			})
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
		continuationToken = output.NextContinuationToken
	}
	return items, nil
}

// GeneratePresignedURL returns a time-limited download URL.
func (b *S3CompatBackend) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presigned, err := b.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presigned.URL, nil
}

// Copy duplicates an object within the same bucket.
func (b *S3CompatBackend) Copy(ctx context.Context, srcKey, destKey string) error {
	_, err := b.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(b.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", b.bucket, srcKey)),
		Key:        aws.String(destKey),
	})
	if err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", srcKey, destKey, err)
	}
	return nil
}

// GetObjectSize returns the size in bytes of the stored object.
func (b *S3CompatBackend) GetObjectSize(ctx context.Context, key string) (int64, error) {
	output, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object meta: %w", err)
	}
	return aws.ToInt64(output.ContentLength), nil
}

// isS3NotFound returns true if the error is an S3 "not found" (404) response.
func isS3NotFound(err error) bool {
	var respErr *awshttp.ResponseError
	if ok := errors.As(err, &respErr); ok {
		return respErr.HTTPStatusCode() == http.StatusNotFound
	}
	return false
}
