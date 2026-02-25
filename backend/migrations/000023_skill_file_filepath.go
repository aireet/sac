package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] renaming skill_files.filename to filepath...")

		_, err := db.ExecContext(ctx, `
			ALTER TABLE skill_files RENAME COLUMN filename TO filepath;
			ALTER TABLE skill_files DROP CONSTRAINT IF EXISTS skill_files_skill_id_filename_key;
			ALTER TABLE skill_files ADD CONSTRAINT skill_files_skill_id_filepath_key UNIQUE (skill_id, filepath);
		`)
		if err != nil {
			return fmt.Errorf("failed to rename filename to filepath: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] reverting filepath back to filename...")

		_, _ = db.ExecContext(ctx, `
			ALTER TABLE skill_files DROP CONSTRAINT IF EXISTS skill_files_skill_id_filepath_key;
			ALTER TABLE skill_files RENAME COLUMN filepath TO filename;
			ALTER TABLE skill_files ADD CONSTRAINT skill_files_skill_id_filename_key UNIQUE (skill_id, filename);
		`)

		fmt.Println("done")
		return nil
	})
}
