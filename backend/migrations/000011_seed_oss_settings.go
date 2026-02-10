package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] seeding OSS settings into system_settings...")

		for _, kv := range []struct{ key, value, desc string }{
			{"oss_endpoint", `""`, "阿里云 OSS Endpoint (例: oss-cn-shanghai.aliyuncs.com)"},
			{"oss_access_key_id", `""`, "阿里云 OSS AccessKey ID"},
			{"oss_access_key_secret", `""`, "阿里云 OSS AccessKey Secret"},
			{"oss_bucket", `""`, "阿里云 OSS Bucket 名称 (例: sac-workspace)"},
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

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing OSS settings from system_settings...")

		_, err := db.ExecContext(ctx, `
			DELETE FROM system_settings
			WHERE key IN ('oss_endpoint', 'oss_access_key_id', 'oss_access_key_secret', 'oss_bucket')
		`)
		if err != nil {
			return fmt.Errorf("failed to remove OSS settings: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
