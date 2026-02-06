package migrations

import (
	"context"
	"fmt"

	"github.com/echotech/sac/internal/models"
	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [up migration] creating agents and agent_skills tables...")

		// Create agents table
		_, err := db.NewCreateTable().
			Model((*models.Agent)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agents table: %w", err)
		}

		// Create agent_skills junction table
		_, err = db.NewCreateTable().
			Model((*models.AgentSkill)(nil)).
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agent_skills table: %w", err)
		}

		// Create indexes for agents
		_, err = db.NewCreateIndex().
			Model((*models.Agent)(nil)).
			Index("idx_agents_created_by").
			Column("created_by").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agents index: %w", err)
		}

		// Create indexes for agent_skills
		_, err = db.NewCreateIndex().
			Model((*models.AgentSkill)(nil)).
			Index("idx_agent_skills_agent_id").
			Column("agent_id").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agent_skills agent_id index: %w", err)
		}

		_, err = db.NewCreateIndex().
			Model((*models.AgentSkill)(nil)).
			Index("idx_agent_skills_skill_id").
			Column("skill_id").
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agent_skills skill_id index: %w", err)
		}

		// Create unique constraint on agent_id + skill_id
		_, err = db.NewCreateIndex().
			Model((*models.AgentSkill)(nil)).
			Index("idx_agent_skills_unique").
			Column("agent_id", "skill_id").
			Unique().
			IfNotExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create agent_skills unique index: %w", err)
		}

		fmt.Println("done")
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] dropping agents and agent_skills tables...")

		// Drop agent_skills table first (foreign key)
		_, err := db.NewDropTable().
			Model((*models.AgentSkill)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to drop agent_skills table: %w", err)
		}

		// Drop agents table
		_, err = db.NewDropTable().
			Model((*models.Agent)(nil)).
			IfExists().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to drop agents table: %w", err)
		}

		fmt.Println("done")
		return nil
	})
}
