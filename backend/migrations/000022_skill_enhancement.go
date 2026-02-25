package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding skill frontmatter and skill_files table...")

		_, err := db.ExecContext(ctx, `
			ALTER TABLE skills ADD COLUMN IF NOT EXISTS frontmatter JSONB NOT NULL DEFAULT '{}';

			CREATE TABLE IF NOT EXISTS skill_files (
				id BIGSERIAL PRIMARY KEY,
				skill_id BIGINT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
				filename TEXT NOT NULL,
				s3_key TEXT NOT NULL,
				size BIGINT NOT NULL DEFAULT 0,
				content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(skill_id, filename)
			);
			CREATE INDEX IF NOT EXISTS idx_skill_files_skill_id ON skill_files(skill_id);
		`)
		if err != nil {
			return fmt.Errorf("failed to add skill enhancement: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing skill frontmatter and skill_files table...")

		_, _ = db.ExecContext(ctx, `
			DROP TABLE IF EXISTS skill_files;
			ALTER TABLE skills DROP COLUMN IF EXISTS frontmatter;
		`)

		fmt.Println("done")
		return nil
	})
}
