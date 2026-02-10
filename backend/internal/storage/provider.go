package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/uptrace/bun"
)

// OSSProvider lazily creates and caches an OSSClient based on system_settings.
// It re-creates the client whenever the stored config changes.
type OSSProvider struct {
	db    *bun.DB
	mu    sync.Mutex
	cache *OSSClient
	// fingerprint of the last config used to create the client
	configHash string
}

// NewOSSProvider creates a provider that reads OSS config from system_settings.
func NewOSSProvider(db *bun.DB) *OSSProvider {
	return &OSSProvider{db: db}
}

type ossConfig struct {
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
}

func (p *OSSProvider) readConfig(ctx context.Context) (*ossConfig, error) {
	type row struct {
		Key   string `bun:"key"`
		Value string `bun:"value"`
	}

	var rows []row
	err := p.db.NewSelect().
		TableExpr("system_settings").
		Column("key", "value").
		Where("key IN (?)", bun.In([]string{
			"oss_endpoint", "oss_access_key_id", "oss_access_key_secret", "oss_bucket",
		})).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("failed to read OSS settings: %w", err)
	}

	cfg := &ossConfig{}
	for _, r := range rows {
		// Values are stored as JSONB strings (e.g. `"value"` with quotes)
		var val string
		if err := json.Unmarshal([]byte(r.Value), &val); err != nil {
			val = r.Value // fallback: use raw value
		}
		switch r.Key {
		case "oss_endpoint":
			cfg.Endpoint = val
		case "oss_access_key_id":
			cfg.AccessKeyID = val
		case "oss_access_key_secret":
			cfg.AccessKeySecret = val
		case "oss_bucket":
			cfg.Bucket = val
		}
	}

	return cfg, nil
}

func (c *ossConfig) hash() string {
	return fmt.Sprintf("%s|%s|%s|%s", c.Endpoint, c.AccessKeyID, c.AccessKeySecret, c.Bucket)
}

func (c *ossConfig) isComplete() bool {
	return c.Endpoint != "" && c.AccessKeyID != "" && c.AccessKeySecret != "" && c.Bucket != ""
}

// GetClient returns a cached OSSClient, creating or refreshing it if config changed.
// Returns nil if OSS is not configured (endpoint empty).
func (p *OSSProvider) GetClient(ctx context.Context) *OSSClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg, err := p.readConfig(ctx)
	if err != nil {
		log.Printf("Warning: failed to read OSS config from settings: %v", err)
		return p.cache // return whatever we had
	}

	if !cfg.isComplete() {
		return nil // OSS not configured
	}

	hash := cfg.hash()
	if p.cache != nil && p.configHash == hash {
		return p.cache // config unchanged, reuse client
	}

	// Config changed or first call — create new client
	client, err := NewOSSClient(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, cfg.Bucket)
	if err != nil {
		log.Printf("Warning: failed to create OSS client from settings: %v", err)
		return nil
	}

	p.cache = client
	p.configHash = hash
	log.Printf("OSS client (re)created from admin settings: endpoint=%s, bucket=%s", cfg.Endpoint, cfg.Bucket)
	return client
}

// IsConfigured returns true if OSS settings are complete (without creating a client).
func (p *OSSProvider) IsConfigured(ctx context.Context) bool {
	cfg, err := p.readConfig(ctx)
	if err != nil {
		return false
	}
	return cfg.isComplete()
}
