package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/pkg/config"
	"g.echo.tech/dev/sac/pkg/logger"
	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("maintenance: failed to load config")
	}

	logger.Init(cfg.LogLevel, cfg.LogFormat)

	log.Info().Msg("maintenance: starting")

	if err := database.Initialize(cfg); err != nil {
		log.Fatal().Err(err).Msg("maintenance: failed to initialize database")
	}
	defer database.Close()

	containerMgr, err := container.NewManager(cfg.KubeconfigPath, cfg.Namespace, cfg.DockerRegistry, cfg.DockerImage, cfg.SidecarImage)
	if err != nil {
		log.Fatal().Err(err).Msg("maintenance: failed to create container manager")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// --- Task 1: Skill sync ---
	syncSkills(ctx, containerMgr)

	// --- Task 2: Conversation history cleanup ---
	cleanupConversations(ctx)

	log.Info().Msg("maintenance: all tasks complete")
}

func syncSkills(ctx context.Context, containerMgr *container.Manager) {
	storageProvider := storage.NewStorageProvider(database.DB)
	syncService := skill.NewSyncService(database.DB, containerMgr, storageProvider)

	var agents []models.Agent
	err := database.DB.NewSelect().Model(&agents).Column("id", "created_by").Scan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("maintenance: skill-sync: failed to list agents")
		return
	}

	log.Info().Int("count", len(agents)).Msg("maintenance: skill-sync: syncing agents")

	var failed int
	for _, a := range agents {
		userID := fmt.Sprintf("%d", a.CreatedBy)
		if err := syncService.SyncAllSkillsToAgent(ctx, userID, a.ID); err != nil {
			log.Error().Err(err).Int64("agent_id", a.ID).Msg("maintenance: skill-sync: agent failed")
			failed++
		}
	}

	log.Info().Int("synced", len(agents)-failed).Int("failed", failed).Msg("maintenance: skill-sync: done")
}

func cleanupConversations(ctx context.Context) {
	retentionDays := 30

	var setting models.SystemSetting
	err := database.DB.NewSelect().Model(&setting).Where("key = ?", "conversation_retention_days").Scan(ctx)
	if err == nil {
		val := strings.Trim(string(setting.Value), "\"")
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			retentionDays = n
		}
	}

	log.Info().Int("retention_days", retentionDays).Msg("maintenance: conversation-cleanup")

	res, err := database.DB.ExecContext(ctx,
		"DELETE FROM conversation_histories WHERE timestamp < NOW() - INTERVAL '1 day' * ?",
		retentionDays,
	)
	if err != nil {
		log.Error().Err(err).Msg("maintenance: conversation-cleanup: failed")
		return
	}

	rows, _ := res.RowsAffected()
	log.Info().Int64("deleted_rows", rows).Msg("maintenance: conversation-cleanup: done")
}
