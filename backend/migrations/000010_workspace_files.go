package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] creating workspace_files and workspace_quotas tables...")

		_, err := db.ExecContext(ctx, `
			CREATE TABLE workspace_files (
				id BIGSERIAL PRIMARY KEY,
				user_id BIGINT NOT NULL DEFAULT 0,
				workspace_type TEXT NOT NULL DEFAULT 'private',
				oss_key TEXT NOT NULL,
				file_name TEXT NOT NULL,
				file_path TEXT NOT NULL,
				content_type TEXT DEFAULT '',
				size_bytes BIGINT NOT NULL DEFAULT 0,
				checksum TEXT DEFAULT '',
				is_directory BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create workspace_files table: %w", err)
		}

		for _, ddl := range []string{
			`CREATE INDEX idx_wf_user_type ON workspace_files (user_id, workspace_type)`,
			`CREATE INDEX idx_wf_file_path ON workspace_files (file_path)`,
			`CREATE UNIQUE INDEX idx_wf_oss_key ON workspace_files (oss_key)`,
		} {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("failed to create index: %w", err)
			}
		}

		_, err = db.ExecContext(ctx, `
			CREATE TABLE workspace_quotas (
				user_id BIGINT PRIMARY KEY,
				used_bytes BIGINT NOT NULL DEFAULT 0,
				max_bytes BIGINT NOT NULL DEFAULT 1073741824,
				file_count INT NOT NULL DEFAULT 0,
				max_file_count INT NOT NULL DEFAULT 1000,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create workspace_quotas table: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping workspace tables...")

		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS workspace_quotas CASCADE`)
		if err != nil {
			return fmt.Errorf("failed to drop workspace_quotas: %w", err)
		}

		_, err = db.ExecContext(ctx, `DROP TABLE IF EXISTS workspace_files CASCADE`)
		if err != nil {
			return fmt.Errorf("failed to drop workspace_files: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
