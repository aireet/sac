package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] enabling TimescaleDB extension...")

		_, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE`)
		if err != nil {
			return fmt.Errorf("failed to enable timescaledb: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping TimescaleDB extension...")

		_, err := db.ExecContext(ctx, `DROP EXTENSION IF EXISTS timescaledb CASCADE`)
		if err != nil {
			return fmt.Errorf("failed to drop timescaledb: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
