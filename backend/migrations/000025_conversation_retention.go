package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] seeding conversation_retention_days and skill_sync_interval settings...")

		_, err := db.ExecContext(ctx, `
			INSERT INTO system_settings (key, value, description, created_at, updated_at)
			VALUES
				('conversation_retention_days', '"30"', 'Number of days to retain conversation history', NOW(), NOW()),
				('skill_sync_interval', '"10m"', 'Interval for periodic skill sync', NOW(), NOW())
			ON CONFLICT (key) DO NOTHING;
		`)
		if err != nil {
			return fmt.Errorf("failed to seed settings: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing conversation_retention_days and skill_sync_interval settings...")

		_, _ = db.ExecContext(ctx, `
			DELETE FROM system_settings WHERE key IN ('conversation_retention_days', 'skill_sync_interval');
		`)

		fmt.Println("done")
		return nil
	})
}
