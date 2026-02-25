package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding checksum column to skill_files...")

		_, err := db.ExecContext(ctx, `
			ALTER TABLE skill_files ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT '';
		`)
		if err != nil {
			return fmt.Errorf("failed to add checksum column to skill_files: %w", err)
		}

		fmt.Println("done")

		fmt.Print(" [up migration] adding content_checksum column to skills...")

		_, err = db.ExecContext(ctx, `
			ALTER TABLE skills ADD COLUMN IF NOT EXISTS content_checksum TEXT NOT NULL DEFAULT '';
		`)
		if err != nil {
			return fmt.Errorf("failed to add content_checksum column to skills: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing checksum column from skill_files...")

		_, err := db.ExecContext(ctx, `
			ALTER TABLE skill_files DROP COLUMN IF EXISTS checksum;
		`)
		if err != nil {
			return fmt.Errorf("failed to drop checksum column: %w", err)
		}

		fmt.Println("done")

		fmt.Print(" [down migration] removing content_checksum column from skills...")

		_, err = db.ExecContext(ctx, `
			ALTER TABLE skills DROP COLUMN IF EXISTS content_checksum;
		`)
		if err != nil {
			return fmt.Errorf("failed to drop content_checksum column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
