package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding groups, group_members, group_workspace_quotas...")

		// Groups table
		_, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS groups (
				id BIGSERIAL PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				owner_id BIGINT NOT NULL REFERENCES users(id),
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(name)
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create groups table: %w", err)
		}

		// Group members table
		_, err = db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS group_members (
				id BIGSERIAL PRIMARY KEY,
				group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role VARCHAR(50) NOT NULL DEFAULT 'member',
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(group_id, user_id)
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create group_members table: %w", err)
		}

		// Group workspace quotas
		_, err = db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS group_workspace_quotas (
				group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
				used_bytes BIGINT NOT NULL DEFAULT 0,
				max_bytes BIGINT NOT NULL DEFAULT 1073741824,
				file_count INT NOT NULL DEFAULT 0,
				max_file_count INT NOT NULL DEFAULT 1000,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create group_workspace_quotas table: %w", err)
		}

		// Indexes
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members (user_id)`)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_groups_owner ON groups (owner_id)`)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing groups, group_members, group_workspace_quotas...")

		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS group_workspace_quotas`)
		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS group_members`)
		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS groups`)

		fmt.Println("done")
		return nil
	})
}
