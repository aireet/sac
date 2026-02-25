package storage

import "github.com/rs/zerolog/log"

// NewS3Backend creates a StorageBackend for AWS S3.
func NewS3Backend(region, accessKeyID, secretAccessKey, bucket string) (*S3CompatBackend, error) {
	log.Info().Str("region", region).Str("bucket", bucket).Msg("creating AWS S3 backend")
	return NewS3CompatBackend(S3CompatConfig{
		Region:          region,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
		UsePathStyle:    false,
	})
}
