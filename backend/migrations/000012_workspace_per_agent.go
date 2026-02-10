package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding agent_id to workspace tables...")

		// Add agent_id column to workspace_files
		_, err := db.ExecContext(ctx, `ALTER TABLE workspace_files ADD COLUMN IF NOT EXISTS agent_id BIGINT NOT NULL DEFAULT 0`)
		if err != nil {
			return fmt.Errorf("failed to add agent_id to workspace_files: %w", err)
		}

		// Add index on (user_id, agent_id, workspace_type)
		_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_wf_user_agent_type ON workspace_files (user_id, agent_id, workspace_type)`)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		// Recreate workspace_quotas with composite PK (user_id, agent_id)
		_, err = db.ExecContext(ctx, `
			ALTER TABLE workspace_quotas ADD COLUMN IF NOT EXISTS agent_id BIGINT NOT NULL DEFAULT 0
		`)
		if err != nil {
			return fmt.Errorf("failed to add agent_id to workspace_quotas: %w", err)
		}

		// Drop old PK and create new composite PK
		_, err = db.ExecContext(ctx, `ALTER TABLE workspace_quotas DROP CONSTRAINT IF EXISTS workspace_quotas_pkey`)
		if err != nil {
			return fmt.Errorf("failed to drop old PK: %w", err)
		}

		_, err = db.ExecContext(ctx, `ALTER TABLE workspace_quotas ADD PRIMARY KEY (user_id, agent_id)`)
		if err != nil {
			return fmt.Errorf("failed to add composite PK: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing agent_id from workspace tables...")

		// Revert workspace_quotas
		_, _ = db.ExecContext(ctx, `ALTER TABLE workspace_quotas DROP CONSTRAINT IF EXISTS workspace_quotas_pkey`)
		_, _ = db.ExecContext(ctx, `ALTER TABLE workspace_quotas DROP COLUMN IF EXISTS agent_id`)
		_, _ = db.ExecContext(ctx, `ALTER TABLE workspace_quotas ADD PRIMARY KEY (user_id)`)

		// Revert workspace_files
		_, _ = db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_wf_user_agent_type`)
		_, _ = db.ExecContext(ctx, `ALTER TABLE workspace_files DROP COLUMN IF EXISTS agent_id`)

		fmt.Println("done")
		return nil
	})
}
