package main

import (
	"context"
	"flag"
	"fmt"

	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/migrations"
	"g.echo.tech/dev/sac/pkg/config"
	"g.echo.tech/dev/sac/pkg/logger"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun/migrate"
)

func main() {
	var action string
	flag.StringVar(&action, "action", "up", "Migration action: up, down, status, seed")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	logger.Init(cfg.LogLevel, cfg.LogFormat)

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize database")
	}
	defer database.Close()

	ctx := context.Background()
	migrator := migrate.NewMigrator(database.DB, migrations.Migrations)

	// Initialize migration tables if needed
	if err := migrator.Init(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize migrator")
	}

	switch action {
	case "up":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to lock migrations")
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Migrate(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to migrate")
		}
		if group.IsZero() {
			log.Info().Msg("no new migrations to run")
		} else {
			log.Info().Msgf("migrated to %s", group)
		}

	case "down":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to lock migrations")
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Rollback(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to rollback")
		}
		if group.IsZero() {
			log.Info().Msg("no migrations to rollback")
		} else {
			log.Info().Msgf("rolled back %s", group)
		}

	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to get migration status")
		}
		fmt.Printf("Migrations: %s\n", ms)

	case "seed":
		seedData(ctx)

	default:
		log.Fatal().Str("action", action).Msg("unknown action")
	}
}

func seedData(ctx context.Context) {
	log.Info().Msg("seeding database")

	// Hash default admin password
	hashedPassword, err := auth.HashPassword("admin123")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to hash password")
	}

	// Create admin user with password
	user := &models.User{
		Username:     "admin",
		Email:        "admin@example.com",
		DisplayName:  "Admin User",
		PasswordHash: hashedPassword,
		Role:         "admin",
	}

	_, err = database.DB.NewInsert().
		Model(user).
		On("CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash, role = EXCLUDED.role").
		Exec(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create user")
	}
	log.Info().Msg("created/updated admin user (password: admin123)")

	log.Info().Msg("database seeding completed")
}
