package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] seeding docker_image setting into system_settings...")

		_, err := db.ExecContext(ctx, `
			INSERT INTO system_settings (key, value, description)
			VALUES (?, ?::jsonb, ?)
			ON CONFLICT (key) DO NOTHING
		`,
			"docker_image",
			`"docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/prod/sac/cc:0.0.20"`,
			"CC 容器默认镜像（完整路径含 registry 和 tag）",
		)
		if err != nil {
			return fmt.Errorf("failed to seed docker_image: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] removing docker_image setting from system_settings...")

		_, err := db.ExecContext(ctx, `
			DELETE FROM system_settings
			WHERE key = 'docker_image'
		`)
		if err != nil {
			return fmt.Errorf("failed to remove docker_image setting: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
