package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding group_id to workspace_files, extending workspace_type...")

		// Add group_id column to workspace_files (nullable, for group workspace files)
		_, err := db.ExecContext(ctx, `ALTER TABLE workspace_files ADD COLUMN IF NOT EXISTS group_id BIGINT DEFAULT NULL`)
		if err != nil {
			return fmt.Errorf("failed to add group_id to workspace_files: %w", err)
		}

		// Add index on group_id + workspace_type
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_wf_group_type ON workspace_files (group_id, workspace_type) WHERE group_id IS NOT NULL`)
		if err != nil {
			return fmt.Errorf("failed to create group index: %w", err)
		}

		// Add index for shared workspace lookups
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_wf_shared ON workspace_files (workspace_type) WHERE workspace_type = 'shared'`)
		if err != nil {
			return fmt.Errorf("failed to create shared index: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing group_id from workspace_files...")

		_, _ = db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_wf_shared`)
		_, _ = db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_wf_group_type`)
		_, _ = db.ExecContext(ctx, `ALTER TABLE workspace_files DROP COLUMN IF EXISTS group_id`)

		fmt.Println("done")
		return nil
	})
}
