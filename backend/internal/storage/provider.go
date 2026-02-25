package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"sync"

	"github.com/uptrace/bun"
)

// StorageProvider lazily creates and caches a StorageBackend based on system_settings.
// It re-creates the backend whenever the stored config changes.
type StorageProvider struct {
	db    *bun.DB
	mu    sync.Mutex
	cache StorageBackend
	// fingerprint of the last config used to create the backend
	configHash string
}

// NewStorageProvider creates a provider that reads storage config from system_settings.
func NewStorageProvider(db *bun.DB) *StorageProvider {
	return &StorageProvider{db: db}
}

// storageConfig holds all settings needed to create any backend.
type storageConfig struct {
	Type StorageType
	// OSS
	OSSEndpoint        string
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	OSSBucket          string
	// S3
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Bucket          string
	// S3-compatible (MinIO, RustFS, etc.)
	S3CompatEndpoint        string
	S3CompatAccessKeyID     string
	S3CompatSecretAccessKey string
	S3CompatBucket          string
	S3CompatUseSSL          string
}

// allSettingKeys lists every system_settings key we may need.
var allSettingKeys = []string{
	"storage_type",
	// OSS
	"oss_endpoint", "oss_access_key_id", "oss_access_key_secret", "oss_bucket",
	// S3
	"s3_region", "s3_access_key_id", "s3_secret_access_key", "s3_bucket",
	// S3-compatible
	"s3compat_endpoint", "s3compat_access_key_id", "s3compat_secret_access_key", "s3compat_bucket", "s3compat_use_ssl",
}

func (p *StorageProvider) readConfig(ctx context.Context) (*storageConfig, error) {
	type row struct {
		Key   string `bun:"key"`
		Value string `bun:"value"`
	}

	var rows []row
	err := p.db.NewSelect().
		TableExpr("system_settings").
		Column("key", "value").
		Where("key IN (?)", bun.In(allSettingKeys)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage settings: %w", err)
	}

	cfg := &storageConfig{Type: TypeOSS} // default to OSS
	for _, r := range rows {
		// Values are stored as JSONB strings (e.g. `"value"` with quotes)
		var val string
		if err := json.Unmarshal([]byte(r.Value), &val); err != nil {
			val = r.Value // fallback: use raw value
		}
		switch r.Key {
		case "storage_type":
			if val != "" {
				cfg.Type = StorageType(val)
			}
		// OSS
		case "oss_endpoint":
			cfg.OSSEndpoint = val
		case "oss_access_key_id":
			cfg.OSSAccessKeyID = val
		case "oss_access_key_secret":
			cfg.OSSAccessKeySecret = val
		case "oss_bucket":
			cfg.OSSBucket = val
		// S3
		case "s3_region":
			cfg.S3Region = val
		case "s3_access_key_id":
			cfg.S3AccessKeyID = val
		case "s3_secret_access_key":
			cfg.S3SecretAccessKey = val
		case "s3_bucket":
			cfg.S3Bucket = val
		// S3-compatible
		case "s3compat_endpoint":
			cfg.S3CompatEndpoint = val
		case "s3compat_access_key_id":
			cfg.S3CompatAccessKeyID = val
		case "s3compat_secret_access_key":
			cfg.S3CompatSecretAccessKey = val
		case "s3compat_bucket":
			cfg.S3CompatBucket = val
		case "s3compat_use_ssl":
			cfg.S3CompatUseSSL = val
		}
	}

	return cfg, nil
}

func (c *storageConfig) hash() string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		c.Type,
		c.OSSEndpoint, c.OSSAccessKeyID, c.OSSAccessKeySecret, c.OSSBucket,
		c.S3Region, c.S3AccessKeyID, c.S3SecretAccessKey, c.S3Bucket,
		c.S3CompatEndpoint, c.S3CompatAccessKeyID, c.S3CompatSecretAccessKey, c.S3CompatBucket, c.S3CompatUseSSL,
	)
}

// isComplete returns true if the active backend has all required fields.
func (c *storageConfig) isComplete() bool {
	switch c.Type {
	case TypeOSS:
		return c.OSSEndpoint != "" && c.OSSAccessKeyID != "" && c.OSSAccessKeySecret != "" && c.OSSBucket != ""
	case TypeS3:
		return c.S3Region != "" && c.S3AccessKeyID != "" && c.S3SecretAccessKey != "" && c.S3Bucket != ""
	case TypeS3Compat:
		return c.S3CompatEndpoint != "" && c.S3CompatAccessKeyID != "" && c.S3CompatSecretAccessKey != "" && c.S3CompatBucket != ""
	default:
		return false
	}
}

// createBackend instantiates the concrete StorageBackend for the given config.
func createBackend(cfg *storageConfig) (StorageBackend, error) {
	switch cfg.Type {
	case TypeOSS:
		return NewOSSBackend(cfg.OSSEndpoint, cfg.OSSAccessKeyID, cfg.OSSAccessKeySecret, cfg.OSSBucket)
	case TypeS3:
		return NewS3Backend(cfg.S3Region, cfg.S3AccessKeyID, cfg.S3SecretAccessKey, cfg.S3Bucket)
	case TypeS3Compat:
		useSSL := cfg.S3CompatUseSSL == "true" || cfg.S3CompatUseSSL == "1"
		return NewS3CompatBackend(S3CompatConfig{
			Endpoint:        cfg.S3CompatEndpoint,
			AccessKeyID:     cfg.S3CompatAccessKeyID,
			SecretAccessKey: cfg.S3CompatSecretAccessKey,
			Bucket:          cfg.S3CompatBucket,
			UsePathStyle:    true,
			ForceHTTP:       !useSSL,
		})
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// GetClient returns a cached StorageBackend, creating or refreshing it if config changed.
// Returns nil if storage is not configured.
func (p *StorageProvider) GetClient(ctx context.Context) StorageBackend {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg, err := p.readConfig(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to read storage config from settings")
		return p.cache // return whatever we had
	}

	if !cfg.isComplete() {
		return nil // storage not configured
	}

	hash := cfg.hash()
	if p.cache != nil && p.configHash == hash {
		return p.cache // config unchanged, reuse client
	}

	// Config changed or first call — create new backend
	backend, err := createBackend(cfg)
	if err != nil {
		log.Warn().Err(err).Str("type", string(cfg.Type)).Msg("failed to create storage backend")
		return nil
	}

	p.cache = backend
	p.configHash = hash
	log.Info().Str("type", string(cfg.Type)).Msg("storage backend (re)created from admin settings")
	return backend
}

// IsConfigured returns true if the active storage backend settings are complete.
func (p *StorageProvider) IsConfigured(ctx context.Context) bool {
	cfg, err := p.readConfig(ctx)
	if err != nil {
		return false
	}
	return cfg.isComplete()
}
