package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] seeding storage backend settings into system_settings...")

		for _, kv := range []struct{ key, value, desc string }{
			// Storage type selector (default: "oss" for backward compatibility)
			{"storage_type", `"oss"`, "存储后端类型 (oss / s3 / s3compat)"},

			// AWS S3
			{"s3_region", `""`, "AWS S3 Region (例: us-east-1)"},
			{"s3_access_key_id", `""`, "AWS S3 Access Key ID"},
			{"s3_secret_access_key", `""`, "AWS S3 Secret Access Key"},
			{"s3_bucket", `""`, "AWS S3 Bucket 名称"},

			// S3-compatible (MinIO, RustFS, Cloudflare R2, etc.)
			{"s3compat_endpoint", `""`, "S3 兼容存储 Endpoint (例: minio.example.com:9000)"},
			{"s3compat_access_key_id", `""`, "S3 兼容存储 Access Key ID"},
			{"s3compat_secret_access_key", `""`, "S3 兼容存储 Secret Access Key"},
			{"s3compat_bucket", `""`, "S3 兼容存储 Bucket 名称"},
			{"s3compat_use_ssl", `"false"`, "S3 兼容存储是否启用 SSL (true / false)"},
		} {
			_, err := db.ExecContext(ctx, `
				INSERT INTO system_settings (key, value, description)
				VALUES (?, ?::jsonb, ?)
				ON CONFLICT (key) DO NOTHING
			`, kv.key, kv.value, kv.desc)
			if err != nil {
				return fmt.Errorf("failed to seed %s: %w", kv.key, err)
			}
		}

		// Clean up old minio_* and rustfs_* keys if they exist
		_, _ = db.ExecContext(ctx, `
			DELETE FROM system_settings
			WHERE key IN (
				'minio_endpoint', 'minio_access_key_id', 'minio_secret_access_key', 'minio_bucket', 'minio_use_ssl',
				'rustfs_endpoint', 'rustfs_access_key_id', 'rustfs_secret_access_key', 'rustfs_bucket', 'rustfs_use_ssl'
			)
		`)

		// Migrate storage_type from old values to new
		_, _ = db.ExecContext(ctx, `
			UPDATE system_settings
			SET value = '"s3compat"'
			WHERE key = 'storage_type' AND value IN ('"minio"', '"rustfs"')
		`)

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing storage backend settings from system_settings...")

		_, err := db.ExecContext(ctx, `
			DELETE FROM system_settings
			WHERE key IN (
				'storage_type',
				's3_region', 's3_access_key_id', 's3_secret_access_key', 's3_bucket',
				's3compat_endpoint', 's3compat_access_key_id', 's3compat_secret_access_key', 's3compat_bucket', 's3compat_use_ssl'
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to remove storage backend settings: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
