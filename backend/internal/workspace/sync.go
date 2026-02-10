package workspace

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"github.com/uptrace/bun"
)

const (
	maxSyncFileSize   = 50 << 20 // 50MB — skip files larger than this
	privateWorkDir    = "/workspace/private"
	publicWorkDir     = "/workspace/public"
	claudeCommandsDir = "/root/.claude/commands"
)

// SyncService syncs workspace files from OSS to agent pods.
type SyncService struct {
	db               *bun.DB
	provider         *storage.OSSProvider
	containerManager *container.Manager
}

// NewSyncService creates a new SyncService.
func NewSyncService(db *bun.DB, provider *storage.OSSProvider, containerManager *container.Manager) *SyncService {
	return &SyncService{
		db:               db,
		provider:         provider,
		containerManager: containerManager,
	}
}

// podName returns the StatefulSet pod name for a user-agent pair.
func (s *SyncService) podName(userID string, agentID int64) string {
	return fmt.Sprintf("claude-code-%s-%d-0", userID, agentID)
}

// SyncWorkspaceToPod syncs both private and public workspace files from OSS to a pod.
// Private workspace is per-agent: users/{userID}/agents/{agentID}/...
func (s *SyncService) SyncWorkspaceToPod(ctx context.Context, userID string, agentID int64) error {
	oss := s.provider.GetClient(ctx)
	if oss == nil {
		log.Println("Workspace sync skipped: OSS not configured")
		return nil
	}

	pod := s.podName(userID, agentID)

	// Create workspace directories
	mkdirCmd := []string{"bash", "-c", fmt.Sprintf(
		"mkdir -p %s %s %s",
		privateWorkDir, publicWorkDir, claudeCommandsDir,
	)}
	if _, _, err := s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil); err != nil {
		return fmt.Errorf("failed to create workspace dirs: %w", err)
	}

	// Sync private workspace (per-agent)
	privatePrefix := fmt.Sprintf("users/%s/agents/%d/", userID, agentID)
	if err := s.syncPrefix(ctx, oss, pod, privatePrefix, privateWorkDir, userID); err != nil {
		log.Printf("Warning: private workspace sync error for user %s agent %d: %v", userID, agentID, err)
	}

	// Sync claude-commands from agent workspace
	commandsPrefix := fmt.Sprintf("users/%s/agents/%d/claude-commands/", userID, agentID)
	if err := s.syncPrefix(ctx, oss, pod, commandsPrefix, claudeCommandsDir, userID); err != nil {
		log.Printf("Warning: claude-commands sync error for user %s agent %d: %v", userID, agentID, err)
	}

	// Sync public workspace
	if err := s.syncPrefix(ctx, oss, pod, "public/", publicWorkDir, ""); err != nil {
		log.Printf("Warning: public workspace sync error: %v", err)
	}

	// Make public workspace read-only
	chmodCmd := []string{"bash", "-c", fmt.Sprintf("chmod -R a-w %s 2>/dev/null || true", publicWorkDir)}
	if _, _, err := s.containerManager.ExecInPod(ctx, pod, chmodCmd, nil); err != nil {
		log.Printf("Warning: failed to set public workspace read-only: %v", err)
	}

	log.Printf("Workspace sync completed for user %s, agent %d", userID, agentID)
	return nil
}

// getActivePodsForAgent returns pod names for active sessions of a user's specific agent.
func (s *SyncService) getActivePodsForAgent(ctx context.Context, userID, agentID int64) ([]string, error) {
	var sessions []models.Session
	err := s.db.NewSelect().Model(&sessions).
		Where("user_id = ? AND agent_id = ?", userID, agentID).
		Where("status IN (?)", bun.In([]string{"running", "idle"})).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	pods := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		if sess.PodName != "" {
			pods = append(pods, sess.PodName)
		}
	}
	return pods, nil
}

// getAllActivePods returns pod names for ALL active sessions (all users).
func (s *SyncService) getAllActivePods(ctx context.Context) ([]string, error) {
	var sessions []models.Session
	err := s.db.NewSelect().Model(&sessions).
		Where("status IN (?)", bun.In([]string{"running", "idle"})).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	pods := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		if sess.PodName != "" {
			pods = append(pods, sess.PodName)
		}
	}
	return pods, nil
}

// SyncFileToPods syncs a single uploaded file to all active pods for the user's agent.
func (s *SyncService) SyncFileToPods(ctx context.Context, userID, agentID int64, ossKey, relPath string) {
	oss := s.provider.GetClient(ctx)
	if oss == nil {
		return
	}

	pods, err := s.getActivePodsForAgent(ctx, userID, agentID)
	if err != nil || len(pods) == 0 {
		return
	}

	body, err := oss.Download(ossKey)
	if err != nil {
		log.Printf("Warning: sync download failed for %s: %v", ossKey, err)
		return
	}
	content, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		log.Printf("Warning: sync read failed for %s: %v", ossKey, err)
		return
	}

	// Determine destination directory based on whether it's a claude-command
	destPath := privateWorkDir + "/" + relPath
	if after, ok := strings.CutPrefix(relPath, "claude-commands/"); ok {
		destPath = claudeCommandsDir + "/" + after
	}

	for _, pod := range pods {
		// Ensure parent directory exists
		dir := destPath[:strings.LastIndex(destPath, "/")]
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s", dir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)

		if err := s.containerManager.WriteFileInPod(ctx, pod, destPath, string(content)); err != nil {
			log.Printf("Warning: sync write %s to pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Synced file %s to pod %s", destPath, pod)
		}
	}
}

// DeleteFileFromPods removes a file from all active pods for the user's agent.
func (s *SyncService) DeleteFileFromPods(ctx context.Context, userID, agentID int64, relPath string) {
	pods, err := s.getActivePodsForAgent(ctx, userID, agentID)
	if err != nil || len(pods) == 0 {
		return
	}

	destPath := privateWorkDir + "/" + relPath
	if after, ok := strings.CutPrefix(relPath, "claude-commands/"); ok {
		destPath = claudeCommandsDir + "/" + after
	}

	for _, pod := range pods {
		rmCmd := []string{"bash", "-c", fmt.Sprintf("rm -rf %s", destPath)}
		if _, _, err := s.containerManager.ExecInPod(ctx, pod, rmCmd, nil); err != nil {
			log.Printf("Warning: delete %s from pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Deleted file %s from pod %s", destPath, pod)
		}
	}
}

// SyncPublicFileToPods syncs a public file to ALL active pods.
func (s *SyncService) SyncPublicFileToPods(ctx context.Context, ossKey, relPath string) {
	oss := s.provider.GetClient(ctx)
	if oss == nil {
		return
	}

	pods, err := s.getAllActivePods(ctx)
	if err != nil || len(pods) == 0 {
		return
	}

	body, err := oss.Download(ossKey)
	if err != nil {
		log.Printf("Warning: public sync download failed for %s: %v", ossKey, err)
		return
	}
	content, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		log.Printf("Warning: public sync read failed for %s: %v", ossKey, err)
		return
	}

	destPath := publicWorkDir + "/" + relPath

	for _, pod := range pods {
		dir := destPath[:strings.LastIndex(destPath, "/")]
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s && chmod a+rx %s", dir, dir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)

		if err := s.containerManager.WriteFileInPod(ctx, pod, destPath, string(content)); err != nil {
			log.Printf("Warning: public sync write %s to pod %s failed: %v", destPath, pod, err)
		} else {
			// Make public file read-only
			chmodCmd := []string{"bash", "-c", fmt.Sprintf("chmod a-w %s", destPath)}
			s.containerManager.ExecInPod(ctx, pod, chmodCmd, nil)
			log.Printf("Synced public file %s to pod %s", destPath, pod)
		}
	}
}

// DeletePublicFileFromPods removes a public file from ALL active pods.
func (s *SyncService) DeletePublicFileFromPods(ctx context.Context, relPath string) {
	pods, err := s.getAllActivePods(ctx)
	if err != nil || len(pods) == 0 {
		return
	}

	destPath := publicWorkDir + "/" + relPath

	for _, pod := range pods {
		rmCmd := []string{"bash", "-c", fmt.Sprintf("rm -rf %s", destPath)}
		if _, _, err := s.containerManager.ExecInPod(ctx, pod, rmCmd, nil); err != nil {
			log.Printf("Warning: delete public %s from pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Deleted public file %s from pod %s", destPath, pod)
		}
	}
}

// syncPrefix downloads all files under an OSS prefix and writes them to destDir in the pod.
func (s *SyncService) syncPrefix(ctx context.Context, oss *storage.OSSClient, pod, prefix, destDir, _ string) error {
	objects, err := oss.ListAll(prefix)
	if err != nil {
		return fmt.Errorf("failed to list OSS prefix %s: %w", prefix, err)
	}

	synced := 0
	for _, obj := range objects {
		if obj.Size > maxSyncFileSize {
			log.Printf("Skipping large file %s (%d bytes > %d limit)", obj.Key, obj.Size, maxSyncFileSize)
			continue
		}

		// Skip directory markers
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}

		// Compute relative path within the destination
		relPath := strings.TrimPrefix(obj.Key, prefix)
		if relPath == "" {
			continue
		}

		destPath := destDir + "/" + relPath

		// Download from OSS
		body, err := oss.Download(obj.Key)
		if err != nil {
			log.Printf("Warning: failed to download %s: %v", obj.Key, err)
			continue
		}

		content, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", obj.Key, err)
			continue
		}

		// Write to pod
		if err := s.containerManager.WriteFileInPod(ctx, pod, destPath, string(content)); err != nil {
			log.Printf("Warning: failed to write %s to pod %s: %v", destPath, pod, err)
			continue
		}

		synced++
	}

	if synced > 0 {
		log.Printf("Synced %d files from OSS prefix %s to %s in pod %s", synced, prefix, destDir, pod)
	}
	return nil
}
