package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] removing system_prompt and model columns from agents table...")

		// Drop system_prompt column
		_, err := db.ExecContext(ctx, "ALTER TABLE agents DROP COLUMN IF EXISTS system_prompt")
		if err != nil {
			return fmt.Errorf("failed to drop system_prompt column: %w", err)
		}

		// Drop model column
		_, err = db.ExecContext(ctx, "ALTER TABLE agents DROP COLUMN IF EXISTS model")
		if err != nil {
			return fmt.Errorf("failed to drop model column: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] restoring system_prompt and model columns to agents table...")

		// Add system_prompt column back
		_, err := db.ExecContext(ctx, `
			ALTER TABLE agents
			ADD COLUMN system_prompt TEXT NOT NULL DEFAULT ''
		`)
		if err != nil {
			return fmt.Errorf("failed to add system_prompt column: %w", err)
		}

		// Add model column back
		_, err = db.ExecContext(ctx, `
			ALTER TABLE agents
			ADD COLUMN model VARCHAR(50) NOT NULL DEFAULT 'sonnet'
		`)
		if err != nil {
			return fmt.Errorf("failed to add model column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
