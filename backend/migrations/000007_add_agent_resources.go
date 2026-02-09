package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding resource columns to agents table...")

		for _, col := range []string{"cpu_request", "cpu_limit", "memory_request", "memory_limit"} {
			_, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS %s VARCHAR`, col))
			if err != nil {
				return fmt.Errorf("failed to add %s column: %w", col, err)
			}
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing resource columns from agents table...")

		for _, col := range []string{"cpu_request", "cpu_limit", "memory_request", "memory_limit"} {
			_, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE agents DROP COLUMN IF EXISTS %s`, col))
			if err != nil {
				return fmt.Errorf("failed to drop %s column: %w", col, err)
			}
		}

		fmt.Println("done")
		return nil
	})
}
