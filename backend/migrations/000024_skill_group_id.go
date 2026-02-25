package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding group_id to skills...")

		_, err := db.ExecContext(ctx, `
			ALTER TABLE skills ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;
			CREATE INDEX IF NOT EXISTS idx_skills_group_id ON skills(group_id) WHERE group_id IS NOT NULL;
		`)
		if err != nil {
			return fmt.Errorf("failed to add group_id to skills: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing group_id from skills...")

		_, _ = db.ExecContext(ctx, `
			DROP INDEX IF EXISTS idx_skills_group_id;
			ALTER TABLE skills DROP COLUMN IF EXISTS group_id;
		`)

		fmt.Println("done")
		return nil
	})
}
