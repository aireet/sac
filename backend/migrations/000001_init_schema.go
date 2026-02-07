package migrations

import (
	"context"
	"fmt"

	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] creating tables...")

		// Create tables
		_, err := db.NewCreateTable().
			Model((*models.User)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateTable().
			Model((*models.Session)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateTable().
			Model((*models.Skill)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateTable().
			Model((*models.ConversationLog)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		// Create indexes
		_, err = db.NewCreateIndex().
			Model((*models.Session)(nil)).
			Index("idx_sessions_user_id").
			Column("user_id").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.Session)(nil)).
			Index("idx_sessions_status").
			Column("status").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.Skill)(nil)).
			Index("idx_skills_category").
			Column("category").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.Skill)(nil)).
			Index("idx_skills_created_by").
			Column("created_by").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.Skill)(nil)).
			Index("idx_skills_is_public").
			Column("is_public").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.ConversationLog)(nil)).
			Index("idx_conversation_logs_user_id").
			Column("user_id").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewCreateIndex().
			Model((*models.ConversationLog)(nil)).
			Index("idx_conversation_logs_session_id").
			Column("session_id").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping tables...")

		// Drop tables in reverse order
		_, err := db.NewDropTable().
			Model((*models.ConversationLog)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewDropTable().
			Model((*models.Skill)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewDropTable().
			Model((*models.Session)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = db.NewDropTable().
			Model((*models.User)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return err
		}

		fmt.Println("done")
		return nil
	})
}
