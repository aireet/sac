package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding registration_mode setting...")

		_, err := db.ExecContext(ctx, `
			INSERT INTO system_settings (key, value, description, created_at, updated_at)
			VALUES ('registration_mode', '"invite"', 'Registration mode: open or invite', NOW(), NOW())
			ON CONFLICT (key) DO NOTHING;
		`)
		if err != nil {
			return fmt.Errorf("failed to add registration_mode setting: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing registration_mode setting...")

		_, _ = db.ExecContext(ctx, `DELETE FROM system_settings WHERE key = 'registration_mode'`)

		fmt.Println("done")
		return nil
	})
}
