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
	groupWorkBaseDir  = "/workspace/group"
	sharedWorkDir     = "/workspace/shared"
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
		"mkdir -p %s %s %s %s %s",
		privateWorkDir, publicWorkDir, groupWorkBaseDir, sharedWorkDir, claudeCommandsDir,
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

	// Sync group workspaces for all groups the user belongs to
	s.syncGroupWorkspacesToPod(ctx, oss, pod, userID)

	// Sync shared workspace (read-only)
	if err := s.syncPrefix(ctx, oss, pod, "shared/", sharedWorkDir, ""); err != nil {
		log.Printf("Warning: shared workspace sync error: %v", err)
	}

	// Make shared workspace read-only
	chmodShared := []string{"bash", "-c", fmt.Sprintf("chmod -R a-w %s 2>/dev/null || true", sharedWorkDir)}
	if _, _, err := s.containerManager.ExecInPod(ctx, pod, chmodShared, nil); err != nil {
		log.Printf("Warning: failed to set shared workspace read-only: %v", err)
	}

	log.Printf("Workspace sync completed for user %s, agent %d", userID, agentID)
	return nil
}

// getActivePodsForAgent returns pod names for active sessions of a user's specific agent.
// Session stores the StatefulSet name (e.g. "claude-code-1-37"), but the actual pod
// name has a "-0" suffix (e.g. "claude-code-1-37-0"), so we append it.
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
			pods = append(pods, sess.PodName+"-0")
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
			pods = append(pods, sess.PodName+"-0")
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

// syncGroupWorkspacesToPod syncs all group workspaces for groups the user belongs to.
func (s *SyncService) syncGroupWorkspacesToPod(ctx context.Context, oss *storage.OSSClient, pod, userID string) {
	// Find groups the user belongs to
	var members []models.GroupMember
	err := s.db.NewSelect().Model(&members).
		Relation("Group", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "name")
		}).
		Where("gm.user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		log.Printf("Warning: failed to find user groups for sync: %v", err)
		return
	}

	for _, m := range members {
		if m.Group == nil {
			continue
		}
		groupDir := fmt.Sprintf("%s/%d", groupWorkBaseDir, m.GroupID)
		groupPrefix := fmt.Sprintf("groups/%d/", m.GroupID)

		// Create group directory
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s", groupDir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)

		if err := s.syncPrefix(ctx, oss, pod, groupPrefix, groupDir, ""); err != nil {
			log.Printf("Warning: group %s workspace sync error: %v", m.Group.Name, err)
		}
	}
}

// getActivePodsForGroupMembers returns pod names for active sessions of all group members.
func (s *SyncService) getActivePodsForGroupMembers(ctx context.Context, groupID int64) ([]string, error) {
	var members []models.GroupMember
	err := s.db.NewSelect().Model(&members).
		Where("group_id = ?", groupID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	userIDs := make([]int64, len(members))
	for i, m := range members {
		userIDs[i] = m.UserID
	}

	var sessions []models.Session
	err = s.db.NewSelect().Model(&sessions).
		Where("user_id IN (?)", bun.In(userIDs)).
		Where("status IN (?)", bun.In([]string{"running", "idle"})).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	pods := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		if sess.PodName != "" {
			pods = append(pods, sess.PodName+"-0")
		}
	}
	return pods, nil
}

// SyncGroupFileToPods syncs a single file to all active pods for group members.
func (s *SyncService) SyncGroupFileToPods(ctx context.Context, groupID int64, ossKey, relPath string) {
	oss := s.provider.GetClient(ctx)
	if oss == nil {
		return
	}

	pods, err := s.getActivePodsForGroupMembers(ctx, groupID)
	if err != nil || len(pods) == 0 {
		return
	}

	body, err := oss.Download(ossKey)
	if err != nil {
		log.Printf("Warning: group sync download failed for %s: %v", ossKey, err)
		return
	}
	content, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		log.Printf("Warning: group sync read failed for %s: %v", ossKey, err)
		return
	}

	destPath := fmt.Sprintf("%s/%d/%s", groupWorkBaseDir, groupID, relPath)

	for _, pod := range pods {
		dir := destPath[:strings.LastIndex(destPath, "/")]
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s", dir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)

		if err := s.containerManager.WriteFileInPod(ctx, pod, destPath, string(content)); err != nil {
			log.Printf("Warning: group sync write %s to pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Synced group file %s to pod %s", destPath, pod)
		}
	}
}

// DeleteGroupFileFromPods removes a group file from all active pods of group members.
func (s *SyncService) DeleteGroupFileFromPods(ctx context.Context, groupID int64, relPath string) {
	pods, err := s.getActivePodsForGroupMembers(ctx, groupID)
	if err != nil || len(pods) == 0 {
		return
	}

	destPath := fmt.Sprintf("%s/%d/%s", groupWorkBaseDir, groupID, relPath)

	for _, pod := range pods {
		rmCmd := []string{"bash", "-c", fmt.Sprintf("rm -rf %s", destPath)}
		if _, _, err := s.containerManager.ExecInPod(ctx, pod, rmCmd, nil); err != nil {
			log.Printf("Warning: delete group %s from pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Deleted group file %s from pod %s", destPath, pod)
		}
	}
}

// SyncSharedFileToPods syncs a shared file to ALL active pods (read-only).
func (s *SyncService) SyncSharedFileToPods(ctx context.Context, ossKey, relPath string) {
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
		log.Printf("Warning: shared sync download failed for %s: %v", ossKey, err)
		return
	}
	content, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		log.Printf("Warning: shared sync read failed for %s: %v", ossKey, err)
		return
	}

	destPath := sharedWorkDir + "/" + relPath

	for _, pod := range pods {
		dir := destPath[:strings.LastIndex(destPath, "/")]
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s && chmod a+rx %s", dir, dir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)

		if err := s.containerManager.WriteFileInPod(ctx, pod, destPath, string(content)); err != nil {
			log.Printf("Warning: shared sync write %s to pod %s failed: %v", destPath, pod, err)
		} else {
			// Make shared file read-only
			chmodCmd := []string{"bash", "-c", fmt.Sprintf("chmod a-w %s", destPath)}
			s.containerManager.ExecInPod(ctx, pod, chmodCmd, nil)
			log.Printf("Synced shared file %s to pod %s", destPath, pod)
		}
	}
}

// DeleteSharedFileFromPods removes a shared file from ALL active pods.
func (s *SyncService) DeleteSharedFileFromPods(ctx context.Context, relPath string) {
	pods, err := s.getAllActivePods(ctx)
	if err != nil || len(pods) == 0 {
		return
	}

	destPath := sharedWorkDir + "/" + relPath

	for _, pod := range pods {
		rmCmd := []string{"bash", "-c", fmt.Sprintf("rm -rf %s", destPath)}
		if _, _, err := s.containerManager.ExecInPod(ctx, pod, rmCmd, nil); err != nil {
			log.Printf("Warning: delete shared %s from pod %s failed: %v", destPath, pod, err)
		} else {
			log.Printf("Deleted shared file %s from pod %s", destPath, pod)
		}
	}
}

// SyncProgress represents the progress of a full workspace sync.
type SyncProgress struct {
	Synced int    `json:"synced"`
	Total  int    `json:"total"`
	File   string `json:"file"`
}

// syncTask represents a single file to sync from OSS to pod.
type syncTask struct {
	ossKey   string
	destPath string
	prefix   string
}

// collectTasks lists all syncable files under a prefix and returns tasks.
func (s *SyncService) collectTasks(oss *storage.OSSClient, prefix, destDir string) ([]syncTask, error) {
	objects, err := oss.ListAll(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list OSS prefix %s: %w", prefix, err)
	}
	var tasks []syncTask
	for _, obj := range objects {
		if obj.Size > maxSyncFileSize || strings.HasSuffix(obj.Key, "/") {
			continue
		}
		relPath := strings.TrimPrefix(obj.Key, prefix)
		if relPath == "" {
			continue
		}
		tasks = append(tasks, syncTask{
			ossKey:   obj.Key,
			destPath: destDir + "/" + relPath,
			prefix:   prefix,
		})
	}
	return tasks, nil
}

// SyncWorkspaceToPodWithProgress syncs with progress callback for SSE.
func (s *SyncService) SyncWorkspaceToPodWithProgress(ctx context.Context, userID string, agentID int64, progressFn func(SyncProgress)) error {
	oss := s.provider.GetClient(ctx)
	if oss == nil {
		log.Println("Workspace sync skipped: OSS not configured")
		return nil
	}

	pod := s.podName(userID, agentID)

	// Create workspace directories
	mkdirCmd := []string{"bash", "-c", fmt.Sprintf(
		"mkdir -p %s %s %s %s %s",
		privateWorkDir, publicWorkDir, groupWorkBaseDir, sharedWorkDir, claudeCommandsDir,
	)}
	if _, _, err := s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil); err != nil {
		return fmt.Errorf("failed to create workspace dirs: %w", err)
	}

	// Collect all tasks
	var allTasks []syncTask

	privatePrefix := fmt.Sprintf("users/%s/agents/%d/", userID, agentID)
	if tasks, err := s.collectTasks(oss, privatePrefix, privateWorkDir); err == nil {
		allTasks = append(allTasks, tasks...)
	}

	commandsPrefix := fmt.Sprintf("users/%s/agents/%d/claude-commands/", userID, agentID)
	if tasks, err := s.collectTasks(oss, commandsPrefix, claudeCommandsDir); err == nil {
		allTasks = append(allTasks, tasks...)
	}

	if tasks, err := s.collectTasks(oss, "public/", publicWorkDir); err == nil {
		allTasks = append(allTasks, tasks...)
	}

	// Group workspaces
	var members []models.GroupMember
	_ = s.db.NewSelect().Model(&members).
		Where("gm.user_id = ?", userID).
		Scan(ctx)
	for _, m := range members {
		groupDir := fmt.Sprintf("%s/%d", groupWorkBaseDir, m.GroupID)
		groupPrefix := fmt.Sprintf("groups/%d/", m.GroupID)
		mkdirCmd := []string{"bash", "-c", fmt.Sprintf("mkdir -p %s", groupDir)}
		s.containerManager.ExecInPod(ctx, pod, mkdirCmd, nil)
		if tasks, err := s.collectTasks(oss, groupPrefix, groupDir); err == nil {
			allTasks = append(allTasks, tasks...)
		}
	}

	if tasks, err := s.collectTasks(oss, "shared/", sharedWorkDir); err == nil {
		allTasks = append(allTasks, tasks...)
	}

	total := len(allTasks)
	progressFn(SyncProgress{Synced: 0, Total: total})

	// Sync all tasks
	synced := 0
	for _, task := range allTasks {
		body, err := oss.Download(task.ossKey)
		if err != nil {
			log.Printf("Warning: failed to download %s: %v", task.ossKey, err)
			continue
		}
		content, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", task.ossKey, err)
			continue
		}
		if err := s.containerManager.WriteFileInPod(ctx, pod, task.destPath, string(content)); err != nil {
			log.Printf("Warning: failed to write %s to pod %s: %v", task.destPath, pod, err)
			continue
		}
		synced++
		// Extract short filename for display
		parts := strings.Split(task.destPath, "/")
		fileName := parts[len(parts)-1]
		progressFn(SyncProgress{Synced: synced, Total: total, File: fileName})
	}

	// Make public/shared workspace read-only
	for _, dir := range []string{publicWorkDir, sharedWorkDir} {
		chmodCmd := []string{"bash", "-c", fmt.Sprintf("chmod -R a-w %s 2>/dev/null || true", dir)}
		s.containerManager.ExecInPod(ctx, pod, chmodCmd, nil)
	}

	log.Printf("Workspace sync completed for user %s, agent %d (%d files)", userID, agentID, synced)
	return nil
}

// syncPrefix downloads all files under an OSS prefix and writes them to destDir in the pod.
func (s *SyncService) syncPrefix(ctx context.Context, oss *storage.OSSClient, pod, prefix, destDir, _ string) error {
	tasks, err := s.collectTasks(oss, prefix, destDir)
	if err != nil {
		return err
	}

	synced := 0
	for _, task := range tasks {
		body, err := oss.Download(task.ossKey)
		if err != nil {
			log.Printf("Warning: failed to download %s: %v", task.ossKey, err)
			continue
		}
		content, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", task.ossKey, err)
			continue
		}
		if err := s.containerManager.WriteFileInPod(ctx, pod, task.destPath, string(content)); err != nil {
			log.Printf("Warning: failed to write %s to pod %s: %v", task.destPath, pod, err)
			continue
		}
		synced++
	}

	if synced > 0 {
		log.Printf("Synced %d files from OSS prefix %s to %s in pod %s", synced, prefix, destDir, pod)
	}
	return nil
}
