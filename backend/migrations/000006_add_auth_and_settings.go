package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding auth fields and settings tables...")

		// Add password_hash column to users table
		_, err := db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255)`)
		if err != nil {
			return fmt.Errorf("failed to add password_hash column: %w", err)
		}

		// Add role column to users table
		_, err = db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(20) DEFAULT 'user' NOT NULL`)
		if err != nil {
			return fmt.Errorf("failed to add role column: %w", err)
		}

		// Create system_settings table
		_, err = db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS system_settings (
				id BIGSERIAL PRIMARY KEY,
				key VARCHAR(255) UNIQUE NOT NULL,
				value JSONB NOT NULL DEFAULT '{}',
				description TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create system_settings table: %w", err)
		}

		// Create user_settings table
		_, err = db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS user_settings (
				id BIGSERIAL PRIMARY KEY,
				user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				key VARCHAR(255) NOT NULL,
				value JSONB NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, key)
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create user_settings table: %w", err)
		}

		// Seed default system settings
		_, err = db.ExecContext(ctx, `
			INSERT INTO system_settings (key, value, description) VALUES
				('max_agents_per_user', '"3"', 'Maximum number of agents per user'),
				('default_cpu_request', '"2"', 'Default CPU request for agent pods'),
				('default_cpu_limit', '"2"', 'Default CPU limit for agent pods'),
				('default_memory_request', '"4Gi"', 'Default memory request for agent pods'),
				('default_memory_limit', '"4Gi"', 'Default memory limit for agent pods')
			ON CONFLICT DO NOTHING
		`)
		if err != nil {
			return fmt.Errorf("failed to seed system settings: %w", err)
		}

		// Set user ID=1 as admin
		_, err = db.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = 1`)
		if err != nil {
			return fmt.Errorf("failed to set admin role: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing auth fields and settings tables...")

		// Drop user_settings table
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS user_settings`)
		if err != nil {
			return fmt.Errorf("failed to drop user_settings table: %w", err)
		}

		// Drop system_settings table
		_, err = db.ExecContext(ctx, `DROP TABLE IF EXISTS system_settings`)
		if err != nil {
			return fmt.Errorf("failed to drop system_settings table: %w", err)
		}

		// Drop role column from users
		_, err = db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN IF EXISTS role`)
		if err != nil {
			return fmt.Errorf("failed to drop role column: %w", err)
		}

		// Drop password_hash column from users
		_, err = db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN IF EXISTS password_hash`)
		if err != nil {
			return fmt.Errorf("failed to drop password_hash column: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
