package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding command_name column to skills table...")

		_, err := db.ExecContext(ctx, `ALTER TABLE skills ADD COLUMN IF NOT EXISTS command_name VARCHAR(100)`)
		if err != nil {
			return fmt.Errorf("failed to add command_name column: %w", err)
		}

		// Create unique partial index (only for non-null values)
		_, err = db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_command_name ON skills (command_name) WHERE command_name IS NOT NULL`)
		if err != nil {
			return fmt.Errorf("failed to create unique index: %w", err)
		}

		// Backfill: use 'skill-<id>' for names that produce empty strings (e.g. Chinese),
		// otherwise convert to kebab-case
		_, err = db.ExecContext(ctx, `
			UPDATE skills
			SET command_name = CASE
				WHEN TRIM(LOWER(REGEXP_REPLACE(REGEXP_REPLACE(TRIM(name), '[^a-zA-Z0-9\s-]', '', 'g'), '\s+', '-', 'g')), '-') = ''
				THEN 'skill-' || id::text
				ELSE LOWER(REGEXP_REPLACE(REGEXP_REPLACE(TRIM(name), '[^a-zA-Z0-9\s-]', '', 'g'), '\s+', '-', 'g'))
			END
			WHERE command_name IS NULL
		`)
		if err != nil {
			return fmt.Errorf("failed to backfill command_name: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing command_name column from skills table...")

		_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_skills_command_name`)
		if err != nil {
			return fmt.Errorf("failed to drop index: %w", err)
		}

		_, err = db.ExecContext(ctx, `ALTER TABLE skills DROP COLUMN IF EXISTS command_name`)
		if err != nil {
			return fmt.Errorf("failed to drop command_name column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
