package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding agent_id column to sessions table...")

		_, err := db.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_id BIGINT DEFAULT 0`)
		if err != nil {
			return fmt.Errorf("failed to add agent_id column: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing agent_id column from sessions table...")

		_, err := db.ExecContext(ctx, `ALTER TABLE sessions DROP COLUMN IF EXISTS agent_id`)
		if err != nil {
			return fmt.Errorf("failed to drop agent_id column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
