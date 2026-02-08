package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/migrations"
	"g.echo.tech/dev/sac/pkg/config"
	"github.com/uptrace/bun/migrate"
)

func main() {
	var action string
	flag.StringVar(&action, "action", "up", "Migration action: up, down, status, seed")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	migrator := migrate.NewMigrator(database.DB, migrations.Migrations)

	// Initialize migration tables if needed
	if err := migrator.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize migrator: %v", err)
	}

	switch action {
	case "up":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatalf("Failed to lock migrations: %v", err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Migrate(ctx)
		if err != nil {
			log.Fatalf("Failed to migrate: %v", err)
		}
		if group.IsZero() {
			log.Println("No new migrations to run")
		} else {
			log.Printf("Migrated to %s\n", group)
		}

	case "down":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatalf("Failed to lock migrations: %v", err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Rollback(ctx)
		if err != nil {
			log.Fatalf("Failed to rollback: %v", err)
		}
		if group.IsZero() {
			log.Println("No migrations to rollback")
		} else {
			log.Printf("Rolled back %s\n", group)
		}

	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}
		fmt.Printf("Migrations: %s\n", ms)

	case "seed":
		seedData(ctx)

	default:
		log.Fatalf("Unknown action: %s", action)
	}
}

func seedData(ctx context.Context) {
	log.Println("Seeding database...")

	// Create test user
	user := &models.User{
		Username:    "admin",
		Email:       "admin@example.com",
		DisplayName: "Admin User",
	}

	_, err := database.DB.NewInsert().
		Model(user).
		On("CONFLICT (username) DO NOTHING").
		Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
	log.Println("Created test user")

	// Get user ID
	err = database.DB.NewSelect().
		Model(user).
		Where("username = ?", "admin").
		Scan(ctx)
	if err != nil {
		log.Fatalf("Failed to get user: %v", err)
	}

	// Skills are now managed via Skill Marketplace - no pre-seeded skills
	log.Println("Database seeding completed!")
}
