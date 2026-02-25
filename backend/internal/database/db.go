package database

import (
	"context"
	"database/sql"
	"fmt"
"github.com/rs/zerolog/log"

	"g.echo.tech/dev/sac/pkg/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

var DB *bun.DB

// Initialize initializes the database connection
func Initialize(cfg *config.Config) error {
	// Create PostgreSQL connection
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	// Create bun.DB instance
	DB = bun.NewDB(sqldb, pgdialect.New())

	// Add query hook for debugging (only in development)
	DB.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))

	// Verify connection
	ctx := context.Background()
	if err := DB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().Msg("database connection established")
	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// HealthCheck performs a health check on the database
func HealthCheck(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}
	return DB.PingContext(ctx)
}
