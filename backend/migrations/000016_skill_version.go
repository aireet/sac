package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding version columns to skills and agent_skills...")

		_, err := db.ExecContext(ctx, `ALTER TABLE skills ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("failed to add version column to skills: %w", err)
		}

		_, err = db.ExecContext(ctx, `ALTER TABLE agent_skills ADD COLUMN IF NOT EXISTS synced_version INT NOT NULL DEFAULT 0`)
		if err != nil {
			return fmt.Errorf("failed to add synced_version column to agent_skills: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing version columns from skills and agent_skills...")

		_, err := db.ExecContext(ctx, `ALTER TABLE agent_skills DROP COLUMN IF EXISTS synced_version`)
		if err != nil {
			return fmt.Errorf("failed to drop synced_version column: %w", err)
		}

		_, err = db.ExecContext(ctx, `ALTER TABLE skills DROP COLUMN IF EXISTS version`)
		if err != nil {
			return fmt.Errorf("failed to drop version column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
