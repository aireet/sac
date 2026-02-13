package storage

import "log"

// NewS3Backend creates a StorageBackend for AWS S3.
func NewS3Backend(region, accessKeyID, secretAccessKey, bucket string) (*S3CompatBackend, error) {
	log.Printf("Creating AWS S3 backend: region=%s, bucket=%s", region, bucket)
	return NewS3CompatBackend(S3CompatConfig{
		Region:          region,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
		UsePathStyle:    false,
	})
}
