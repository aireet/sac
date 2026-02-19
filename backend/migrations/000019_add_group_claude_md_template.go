package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding claude_md_template column to groups...")

		_, err := db.ExecContext(ctx, `ALTER TABLE groups ADD COLUMN IF NOT EXISTS claude_md_template TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add claude_md_template column: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing claude_md_template column from groups...")

		_, _ = db.ExecContext(ctx, `ALTER TABLE groups DROP COLUMN IF EXISTS claude_md_template`)

		fmt.Println("done")
		return nil
	})
}
