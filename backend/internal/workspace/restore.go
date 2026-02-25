package workspace

import (
	"context"
	"fmt"
	"io"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
)

// RestoreOutputFiles downloads output files from S3 and writes them back into the pod.
// Called during session creation to restore workspace state after pod restart.
func RestoreOutputFiles(ctx context.Context, db *bun.DB, provider *storage.StorageProvider, cm *container.Manager, userID, agentID int64) error {
	backend := provider.GetClient(ctx)
	if backend == nil {
		return nil // storage not configured, nothing to restore
	}

	var files []models.WorkspaceFile
	err := db.NewSelect().
		Model(&files).
		Where("user_id = ?", userID).
		Where("agent_id = ?", agentID).
		Where("workspace_type = ?", "output").
		Where("is_directory = ?", false).
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("query output files: %w", err)
	}

	if len(files) == 0 {
		return nil
	}

	userIDStr := fmt.Sprintf("%d", userID)
	podName := fmt.Sprintf("claude-code-%s-%d-0", userIDStr, agentID)

	log.Info().Int64("user_id", userID).Int64("agent_id", agentID).Int("count", len(files)).Msg("restoring output files to pod")

	var restored int
	for _, f := range files {
		body, err := backend.Download(ctx, f.OSSKey)
		if err != nil {
			log.Warn().Err(err).Str("key", f.OSSKey).Msg("skip: failed to download output file")
			continue
		}

		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			log.Warn().Err(err).Str("key", f.OSSKey).Msg("skip: failed to read output file")
			continue
		}

		podPath := fmt.Sprintf("/workspace/output/%s", f.FilePath)
		if err := cm.WriteFileInPod(ctx, podName, podPath, string(data)); err != nil {
			log.Warn().Err(err).Str("path", podPath).Msg("skip: failed to write output file to pod")
			continue
		}
		restored++
	}

	log.Info().Int("restored", restored).Int("total", len(files)).Msg("output file restore complete")
	return nil
}
