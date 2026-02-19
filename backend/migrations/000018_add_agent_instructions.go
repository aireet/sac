package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] adding agent instructions column and system instructions setting...")

		// Add instructions column to agents table
		_, err := db.ExecContext(ctx, `ALTER TABLE agents ADD COLUMN IF NOT EXISTS instructions TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add instructions column: %w", err)
		}

		// Seed system-level agent instructions
		_, err = db.ExecContext(ctx, `
			INSERT INTO system_settings (key, value, description)
			VALUES (?, ?::jsonb, ?)
			ON CONFLICT (key) DO NOTHING
		`,
			"agent_system_instructions",
			`"You are a Claude Code Agent running inside a sandboxed container.\n\n## Output File Rules\nWhen you generate files intended for the user to view (HTML pages, charts, reports, CSV exports, images, or any other visual artifacts), you MUST save them to the /workspace/output/ directory.\nThis directory is automatically monitored and synced to the user's browser in real time.\n- Source code, config files, and other development artifacts do NOT belong in output\n- Only save \"finished deliverables the user needs to view or download\" to output\n- If /workspace/output/ does not exist, create it first with mkdir -p"`,
			"系统级 Agent 指令 (写入每个 Pod 的 CLAUDE.md，用户不可修改)",
		)
		if err != nil {
			return fmt.Errorf("failed to seed agent_system_instructions: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing agent instructions column and system instructions setting...")

		_, _ = db.ExecContext(ctx, `ALTER TABLE agents DROP COLUMN IF EXISTS instructions`)
		_, _ = db.ExecContext(ctx, `DELETE FROM system_settings WHERE key = 'agent_system_instructions'`)

		fmt.Println("done")
		return nil
	})
}
