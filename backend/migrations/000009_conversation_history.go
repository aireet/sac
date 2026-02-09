package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] creating conversation_histories hypertable...")

		_, err := db.ExecContext(ctx, `
			CREATE TABLE conversation_histories (
				id BIGSERIAL,
				user_id BIGINT NOT NULL,
				agent_id BIGINT NOT NULL,
				session_id TEXT NOT NULL,
				role TEXT NOT NULL,
				content TEXT NOT NULL,
				message_uuid TEXT,
				timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (timestamp, id)
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create conversation_histories table: %w", err)
		}

		_, err = db.ExecContext(ctx, `SELECT create_hypertable('conversation_histories', 'timestamp', if_not_exists => TRUE)`)
		if err != nil {
			return fmt.Errorf("failed to create hypertable: %w", err)
		}

		for _, ddl := range []string{
			`CREATE INDEX idx_ch_user_id ON conversation_histories (user_id, timestamp DESC)`,
			`CREATE INDEX idx_ch_agent_id ON conversation_histories (agent_id, timestamp DESC)`,
			`CREATE INDEX idx_ch_session_id ON conversation_histories (session_id, timestamp DESC)`,
		} {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("failed to create index: %w", err)
			}
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping conversation_histories table...")

		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS conversation_histories CASCADE`)
		if err != nil {
			return fmt.Errorf("failed to drop conversation_histories: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
