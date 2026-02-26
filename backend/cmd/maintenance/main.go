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
	"github.com/uptrace/bun"
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

	// --- Task 3: Stale session cleanup ---
	cleanupStaleSessions(ctx)

	// --- Task 4: Orphaned workspace files cleanup (deleted agents) ---
	storageProvider := storage.NewStorageProvider(database.DB)
	cleanupOrphanedWorkspaceFiles(ctx, storageProvider)

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

func cleanupStaleSessions(ctx context.Context) {
	staleThreshold := 24 * time.Hour

	var setting models.SystemSetting
	err := database.DB.NewSelect().Model(&setting).Where("key = ?", "session_stale_hours").Scan(ctx)
	if err == nil {
		val := strings.Trim(string(setting.Value), "\"")
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			staleThreshold = time.Duration(n) * time.Hour
		}
	}

	cutoff := time.Now().Add(-staleThreshold)
	log.Info().Time("cutoff", cutoff).Msg("maintenance: session-cleanup")

	res, err := database.DB.NewDelete().
		Model((*models.Session)(nil)).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusStopped),
			string(models.SessionStatusDeleted),
			string(models.SessionStatusIdle),
		})).
		Where("updated_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		log.Error().Err(err).Msg("maintenance: session-cleanup: failed")
		return
	}

	rows, _ := res.RowsAffected()
	log.Info().Int64("deleted_sessions", rows).Msg("maintenance: session-cleanup: done")
}

func cleanupOrphanedWorkspaceFiles(ctx context.Context, storageProvider *storage.StorageProvider) {
	// Find workspace_files whose agent_id no longer exists in agents table
	var orphans []models.WorkspaceFile
	err := database.DB.NewSelect().
		Model(&orphans).
		Where("agent_id > 0").
		Where("agent_id NOT IN (SELECT id FROM agents)").
		Scan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("maintenance: orphan-cleanup: failed to query")
		return
	}

	if len(orphans) == 0 {
		log.Info().Msg("maintenance: orphan-cleanup: no orphaned files")
		return
	}

	log.Info().Int("count", len(orphans)).Msg("maintenance: orphan-cleanup: found orphaned files")

	// Delete from S3
	var s3Deleted int
	backend := storageProvider.GetClient(ctx)
	if backend != nil {
		for _, f := range orphans {
			if f.OSSKey == "" {
				continue
			}
			if err := backend.Delete(ctx, f.OSSKey); err != nil {
				log.Warn().Err(err).Str("oss_key", f.OSSKey).Msg("maintenance: orphan-cleanup: failed to delete from S3")
			} else {
				s3Deleted++
			}
		}
	}

	// Delete DB records
	ids := make([]int64, len(orphans))
	for i, f := range orphans {
		ids[i] = f.ID
	}
	res, err := database.DB.NewDelete().
		Model((*models.WorkspaceFile)(nil)).
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	if err != nil {
		log.Error().Err(err).Msg("maintenance: orphan-cleanup: failed to delete DB records")
		return
	}

	dbRows, _ := res.RowsAffected()

	// Clean up orphaned quota records
	quotaRes, err := database.DB.NewDelete().
		Model((*models.WorkspaceQuota)(nil)).
		Where("agent_id > 0").
		Where("agent_id NOT IN (SELECT id FROM agents)").
		Exec(ctx)
	var quotaRows int64
	if err != nil {
		log.Warn().Err(err).Msg("maintenance: orphan-cleanup: failed to delete orphaned quotas")
	} else {
		quotaRows, _ = quotaRes.RowsAffected()
	}

	log.Info().Int("s3_deleted", s3Deleted).Int64("db_deleted", dbRows).Int64("quotas_deleted", quotaRows).Msg("maintenance: orphan-cleanup: done")
}
