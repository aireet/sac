package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] creating shared_links table...")

		_, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS shared_links (
				id         BIGSERIAL PRIMARY KEY,
				short_code VARCHAR(10) NOT NULL UNIQUE,
				user_id    BIGINT NOT NULL,
				agent_id   BIGINT NOT NULL,
				file_path  TEXT NOT NULL,
				oss_key    TEXT NOT NULL,
				file_name  TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_shared_links_code ON shared_links (short_code);
			CREATE INDEX IF NOT EXISTS idx_shared_links_user ON shared_links (user_id, agent_id);
		`)
		if err != nil {
			return fmt.Errorf("failed to create shared_links table: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping shared_links table...")

		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS shared_links`)

		fmt.Println("done")
		return nil
	})
}
