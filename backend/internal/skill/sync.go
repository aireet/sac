package skill

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
	"gopkg.in/yaml.v3"
)

const (
	skillsDir   = "/root/.claude/skills"
	commandsDir = "/root/.claude/commands" // legacy, cleaned up on sync
)

// SyncService handles syncing skill directories to agent pods.
type SyncService struct {
	db               *bun.DB
	containerManager *container.Manager
	storage          *storage.StorageProvider
	publisher        SyncProgressPublisher
}

// NewSyncService creates a new SyncService.
func NewSyncService(db *bun.DB, containerManager *container.Manager, storageProvider *storage.StorageProvider) *SyncService {
	return &SyncService{
		db:               db,
		containerManager: containerManager,
		storage:          storageProvider,
	}
}

// SetPublisher sets the progress publisher for real-time sync notifications.
// If publisher is nil, progress events are silently dropped.
func (s *SyncService) SetPublisher(publisher SyncProgressPublisher) {
	s.publisher = publisher
}

// publish is a nil-safe helper that sends a progress event.
func (s *SyncService) publish(ctx context.Context, userID int64, agentID int64, event SkillSyncEvent) {
	if s.publisher != nil {
		s.publisher.Publish(ctx, userID, agentID, event)
	}
}

// podName returns the StatefulSet pod name for a user-agent pair.
func (s *SyncService) podName(userID string, agentID int64) string {
	return fmt.Sprintf("claude-code-%s-%d-0", userID, agentID)
}

// md5Hex computes the MD5 hex digest of a string.
func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// bundleS3Key returns the S3 key for a skill's pre-built tar bundle.
func bundleS3Key(skillID int64) string {
	return fmt.Sprintf("skills/%d/bundle.tar", skillID)
}

// RebuildSkillBundle recomputes content_checksum, builds a tar bundle of
// SKILL.md + attached files + .checksum, uploads to S3, and bumps version.
// Called on every skill content change (update, file upload/edit/delete).
func (s *SyncService) RebuildSkillBundle(ctx context.Context, skillID int64) error {
	var sk models.Skill
	if err := s.db.NewSelect().Model(&sk).Where("id = ?", skillID).Scan(ctx); err != nil {
		return fmt.Errorf("failed to load skill %d: %w", skillID, err)
	}

	skillMD := buildSkillMD(&sk)

	var files []models.SkillFile
	_ = s.db.NewSelect().Model(&files).
		Where("skill_id = ?", skillID).
		Order("filepath ASC").
		Scan(ctx)

	// Compute content_checksum: md5(SKILL.md + \x00 + sorted filepath:checksum lines)
	ch := md5.New()
	ch.Write([]byte(skillMD))
	ch.Write([]byte{0x00})
	for _, f := range files {
		ch.Write([]byte(f.Filepath + ":" + f.Checksum + "\n"))
	}
	checksum := hex.EncodeToString(ch.Sum(nil))

	// Build tar archive in memory
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	// SKILL.md
	skillMDBytes := []byte(skillMD)
	_ = tw.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0644, Size: int64(len(skillMDBytes))})
	_, _ = tw.Write(skillMDBytes)

	// Attached files from S3
	if len(files) > 0 && s.storage != nil {
		if backend := s.storage.GetClient(ctx); backend != nil {
			for _, f := range files {
				reader, err := backend.Download(ctx, f.S3Key)
				if err != nil {
					log.Warn().Err(err).Str("filepath", f.Filepath).Msg("RebuildSkillBundle: failed to download from S3")
					continue
				}
				data, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					log.Warn().Err(err).Str("filepath", f.Filepath).Msg("RebuildSkillBundle: failed to read file")
					continue
				}
				_ = tw.WriteHeader(&tar.Header{Name: f.Filepath, Mode: 0644, Size: int64(len(data))})
				_, _ = tw.Write(data)
			}
		}
	}

	// .checksum marker file
	checksumBytes := []byte(checksum)
	_ = tw.WriteHeader(&tar.Header{Name: ".checksum", Mode: 0644, Size: int64(len(checksumBytes))})
	_, _ = tw.Write(checksumBytes)
	_ = tw.Close()

	// Upload bundle.tar to S3
	if s.storage != nil {
		if backend := s.storage.GetClient(ctx); backend != nil {
			tarData := tarBuf.Bytes()
			if err := backend.Upload(ctx, bundleS3Key(skillID), bytes.NewReader(tarData), int64(len(tarData)), "application/x-tar"); err != nil {
				log.Warn().Err(err).Int64("skill_id", skillID).Msg("RebuildSkillBundle: failed to upload bundle.tar")
				// Non-fatal: sync will fall back to building tar on the fly
			}
		}
	}

	// Update DB: content_checksum + version bump
	_, err := s.db.NewUpdate().
		Model((*models.Skill)(nil)).
		Set("content_checksum = ?", checksum).
		Set("version = version + 1").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", skillID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update content_checksum for skill %d: %w", skillID, err)
	}

	log.Debug().Int64("skill_id", skillID).Str("checksum", checksum).Msg("rebuilt skill bundle")
	return nil
}

// buildSkillMD assembles a SKILL.md file with optional YAML frontmatter.
// Always injects user_invocable: true if not explicitly set, so Claude Code
// registers the skill as a callable /command.
func buildSkillMD(sk *models.Skill) string {
	var b strings.Builder

	fm := make(map[string]any)

	if !sk.Frontmatter.IsZero() {
		f := &sk.Frontmatter

		if len(f.AllowedTools) > 0 {
			fm["allowed_tools"] = f.AllowedTools
		}
		if f.Model != "" {
			fm["model"] = f.Model
		}
		if f.Context != "" {
			fm["context"] = f.Context
		}
		if f.Agent != "" {
			fm["agent"] = f.Agent
		}
		if f.DisableModelInvocation {
			fm["disable_model_invocation"] = true
		}
		if f.ArgumentHint != "" {
			fm["argument_hint"] = f.ArgumentHint
		}
		if f.UserInvocable != nil {
			fm["user_invocable"] = *f.UserInvocable
		}
	}

	// Default: user_invocable = true so /command works
	if _, ok := fm["user_invocable"]; !ok {
		fm["user_invocable"] = true
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err == nil && len(yamlBytes) > 0 {
		b.WriteString("---\n")
		b.Write(yamlBytes)
		b.WriteString("---\n")
	}

	b.WriteString(sk.Prompt)
	return b.String()
}

// readPodChecksum reads the .checksum file from a skill directory in the pod.
// Returns "" if the file doesn't exist or can't be read (triggers full sync).
func (s *SyncService) readPodChecksum(ctx context.Context, pod, commandName string) string {
	checksumPath := fmt.Sprintf("%s/%s/.checksum", skillsDir, commandName)
	cmd := []string{"cat", checksumPath}
	stdout, _, err := s.containerManager.ExecInPod(ctx, pod, cmd, nil)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// SyncSkillToAgent syncs a single skill to an agent pod.
// Compares content_checksum from DB with the .checksum file on the pod;
// if they match, the skill is skipped. Otherwise, downloads the pre-built
// bundle.tar from S3 and extracts it in one ExecInPod call.
func (s *SyncService) SyncSkillToAgent(ctx context.Context, userID string, agentID int64, sk *models.Skill) error {
	if sk.CommandName == "" {
		return fmt.Errorf("skill %d has no command_name", sk.ID)
	}

	uid, _ := strconv.ParseInt(userID, 10, 64)
	pod := s.podName(userID, agentID)

	// Compare checksums — skip if unchanged
	podChecksum := s.readPodChecksum(ctx, pod, sk.CommandName)
	if sk.ContentChecksum != "" && podChecksum == sk.ContentChecksum {
		log.Debug().Str("command", sk.CommandName).Str("pod", pod).Msg("skill checksum matches, skipping")
		_, _ = s.db.NewUpdate().
			Model((*models.AgentSkill)(nil)).
			Set("synced_version = ?", sk.Version).
			Where("agent_id = ?", agentID).
			Where("skill_id = ?", sk.ID).
			Exec(ctx)
		return nil
	}

	s.publish(ctx, uid, agentID, SkillSyncEvent{
		Action: "progress", SkillID: sk.ID, SkillName: sk.Name,
		CommandName: sk.CommandName, AgentID: agentID,
		Step: "syncing_skill", Message: fmt.Sprintf("Syncing %s...", sk.Name),
	})

	// Download pre-built bundle.tar from S3
	var tarReader io.Reader
	if s.storage != nil {
		if backend := s.storage.GetClient(ctx); backend != nil {
			reader, err := backend.Download(ctx, bundleS3Key(sk.ID))
			if err == nil {
				data, readErr := io.ReadAll(reader)
				reader.Close()
				if readErr == nil && len(data) > 0 {
					tarReader = bytes.NewReader(data)
				}
			}
		}
	}

	// Fallback: build tar on the fly if bundle.tar not available (legacy skills)
	if tarReader == nil {
		log.Debug().Str("command", sk.CommandName).Msg("bundle.tar not found, building on the fly")
		buf, err := s.buildTarOnTheFly(ctx, sk)
		if err != nil {
			return fmt.Errorf("failed to build tar for skill %q: %w", sk.CommandName, err)
		}
		tarReader = buf
	}

	// Clear existing directory and extract tar in one go
	skillDir := fmt.Sprintf("%s/%s", skillsDir, sk.CommandName)
	cmd := []string{"bash", "-c", fmt.Sprintf("rm -rf %s && mkdir -p %s && tar xf - -C %s", skillDir, skillDir, skillDir)}
	_, stderr, err := s.containerManager.ExecInPod(ctx, pod, cmd, tarReader)
	if err != nil {
		return fmt.Errorf("failed to extract tar for skill %q in pod %s: %w (stderr: %s)", sk.CommandName, pod, err, stderr)
	}

	// Update synced_version
	_, _ = s.db.NewUpdate().
		Model((*models.AgentSkill)(nil)).
		Set("synced_version = ?", sk.Version).
		Where("agent_id = ?", agentID).
		Where("skill_id = ?", sk.ID).
		Exec(ctx)

	log.Info().Str("command", sk.CommandName).Int("version", sk.Version).Str("pod", pod).Msg("synced skill via tar")
	return nil
}

// buildTarOnTheFly constructs a tar archive for a skill when no pre-built bundle exists.
// Used as fallback for legacy skills that were created before bundle.tar was introduced.
func (s *SyncService) buildTarOnTheFly(ctx context.Context, sk *models.Skill) (*bytes.Reader, error) {
	skillMD := buildSkillMD(sk)
	contentChecksum := sk.ContentChecksum
	if contentChecksum == "" {
		contentChecksum = md5Hex(skillMD)
	}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	// SKILL.md
	skillMDBytes := []byte(skillMD)
	_ = tw.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0644, Size: int64(len(skillMDBytes))})
	_, _ = tw.Write(skillMDBytes)

	// Attached files from S3
	var files []models.SkillFile
	_ = s.db.NewSelect().Model(&files).Where("skill_id = ?", sk.ID).Order("filepath ASC").Scan(ctx)

	if len(files) > 0 && s.storage != nil {
		if backend := s.storage.GetClient(ctx); backend != nil {
			for _, f := range files {
				reader, err := backend.Download(ctx, f.S3Key)
				if err != nil {
					log.Warn().Err(err).Str("filepath", f.Filepath).Msg("fallback tar: failed to download")
					continue
				}
				data, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					continue
				}
				_ = tw.WriteHeader(&tar.Header{Name: f.Filepath, Mode: 0644, Size: int64(len(data))})
				_, _ = tw.Write(data)
			}
		}
	}

	// .checksum
	checksumBytes := []byte(contentChecksum)
	_ = tw.WriteHeader(&tar.Header{Name: ".checksum", Mode: 0644, Size: int64(len(checksumBytes))})
	_, _ = tw.Write(checksumBytes)
	_ = tw.Close()

	return bytes.NewReader(tarBuf.Bytes()), nil
}

// RemoveSkillFromAgent deletes a skill's directory from the agent pod.
func (s *SyncService) RemoveSkillFromAgent(ctx context.Context, userID string, agentID int64, commandName string) error {
	if commandName == "" {
		return nil
	}

	pod := s.podName(userID, agentID)

	// Remove new-format skill directory
	skillDir := fmt.Sprintf("%s/%s", skillsDir, commandName)
	cmd := []string{"rm", "-rf", skillDir}
	if _, _, err := s.containerManager.ExecInPod(ctx, pod, cmd, nil); err != nil {
		log.Warn().Err(err).Str("dir", skillDir).Str("pod", pod).Msg("failed to remove skill dir")
	}

	// Also remove legacy .claude/commands file if it exists
	legacyPath := fmt.Sprintf("%s/%s.md", commandsDir, commandName)
	_ = s.containerManager.DeleteFileInPod(ctx, pod, legacyPath)

	log.Info().Str("command", commandName).Str("pod", pod).Msg("removed skill")
	return nil
}

// SyncAllSkillsToAgent syncs all installed skills for an agent to its pod.
// Uses two-layer incremental strategy:
//  1. Version skip: if synced_version == skill.version, skip the entire skill
//  2. Content checksum comparison: for changed skills, compare DB checksum with pod checksum
func (s *SyncService) SyncAllSkillsToAgent(ctx context.Context, userID string, agentID int64) error {
	uid, _ := strconv.ParseInt(userID, 10, 64)

	// Query skills with their agent_skills junction to get synced_version
	type skillWithSync struct {
		models.Skill
		SyncedVersion int `bun:"synced_version"`
	}

	var skills []skillWithSync
	err := s.db.NewSelect().
		TableExpr("skills AS sk").
		Join("JOIN agent_skills AS ags ON ags.skill_id = sk.id").
		ColumnExpr("sk.*").
		ColumnExpr("ags.synced_version").
		Where("ags.agent_id = ?", agentID).
		Scan(ctx, &skills)

	if err != nil {
		return fmt.Errorf("failed to query skills for agent %d: %w", agentID, err)
	}

	pod := s.podName(userID, agentID)

	// Lazy revocation: filter out skills the user can no longer access
	var userGroupIDs []int64
	_ = s.db.NewSelect().TableExpr("group_members").
		Column("group_id").
		Where("user_id = ?", uid).
		Scan(ctx, &userGroupIDs)
	groupSet := make(map[int64]bool, len(userGroupIDs))
	for _, gid := range userGroupIDs {
		groupSet[gid] = true
	}

	var visible []skillWithSync
	for i := range skills {
		sk := &skills[i].Skill
		if sk.IsOfficial || sk.IsPublic || sk.CreatedBy == uid {
			visible = append(visible, skills[i])
		} else if sk.GroupID != nil && groupSet[*sk.GroupID] {
			visible = append(visible, skills[i])
		} else {
			// Revoked: remove agent_skills record; stale cleanup below removes pod dir
			_, _ = s.db.NewDelete().Model((*models.AgentSkill)(nil)).
				Where("agent_id = ? AND skill_id = ?", agentID, sk.ID).Exec(ctx)
			log.Info().Int64("skill_id", sk.ID).Str("command", sk.CommandName).Str("pod", pod).Msg("revoked inaccessible skill")
		}
	}
	skills = visible

	// Detect if skills dir exists on pod. If not (pod restart, emptyDir wiped),
	// force full sync by ignoring version skip.
	forceSync := false
	checkCmd := []string{"test", "-d", skillsDir}
	_, _, testErr := s.containerManager.ExecInPod(ctx, pod, checkCmd, nil)
	if testErr != nil {
		forceSync = true
		log.Info().Str("pod", pod).Msg("skills dir missing on pod, forcing full sync")
	}

	// Build set of expected skill directory names
	expectedDirs := make(map[string]bool)

	var synced, skippedByVersion int

	for i := range skills {
		sws := &skills[i]
		sk := &sws.Skill
		if sk.CommandName == "" {
			continue
		}
		expectedDirs[sk.CommandName] = true

		// Layer 1: Version skip — if synced_version matches AND pod has files, skip
		if !forceSync && sws.SyncedVersion == sk.Version {
			skippedByVersion++
			continue
		}

		// Needs sync — SyncSkillToAgent handles checksum comparison internally
		if err := s.SyncSkillToAgent(ctx, userID, agentID, sk); err != nil {
			log.Warn().Err(err).Str("command", sk.CommandName).Str("pod", pod).Msg("failed to sync skill")
			continue
		}
		synced++
	}

	if skippedByVersion > 0 {
		s.publish(ctx, uid, agentID, SkillSyncEvent{
			Action: "progress", AgentID: agentID,
			Step:    "up_to_date",
			Message: fmt.Sprintf("%d skills up to date, skipping", skippedByVersion),
		})
	}

	// Clean up stale skill directories
	s.publish(ctx, uid, agentID, SkillSyncEvent{
		Action: "progress", AgentID: agentID,
		Step: "cleaning_stale", Message: "Cleaning up stale skills...",
	})

	existingDirs, err := s.containerManager.ListFilesInPod(ctx, pod, skillsDir)
	if err != nil {
		log.Warn().Err(err).Str("pod", pod).Msg("failed to list skills dir")
	} else {
		for _, d := range existingDirs {
			if d == "" {
				continue
			}
			if !expectedDirs[d] {
				dirPath := fmt.Sprintf("%s/%s", skillsDir, d)
				cmd := []string{"rm", "-rf", dirPath}
				if _, _, err := s.containerManager.ExecInPod(ctx, pod, cmd, nil); err != nil {
					log.Warn().Err(err).Str("dir", d).Str("pod", pod).Msg("failed to clean up stale skill dir")
				} else {
					log.Debug().Str("dir", d).Str("pod", pod).Msg("cleaned up stale skill directory")
				}
			}
		}
	}

	// Clean up legacy .claude/commands/*.md files (one-time migration)
	legacyFiles, err := s.containerManager.ListFilesInPod(ctx, pod, commandsDir)
	if err == nil {
		for _, f := range legacyFiles {
			if strings.HasSuffix(f, ".md") {
				filePath := fmt.Sprintf("%s/%s", commandsDir, f)
				_ = s.containerManager.DeleteFileInPod(ctx, pod, filePath)
				log.Debug().Str("file", f).Str("pod", pod).Msg("cleaned up legacy command file")
			}
		}
	}

	s.publish(ctx, uid, agentID, SkillSyncEvent{
		Action: "complete", AgentID: agentID,
		Step:    "done",
		Message: fmt.Sprintf("Sync complete: %d synced, %d up to date", synced, skippedByVersion),
	})

	log.Info().Int("synced", synced).Int("skipped", skippedByVersion).Int("total", len(skills)).Str("pod", pod).Msg("synced skills")
	return nil
}

// CopySkillFiles duplicates all attached files from one skill to another in S3 and DB.
// Used during fork operations.
func (s *SyncService) CopySkillFiles(ctx context.Context, srcSkillID, dstSkillID int64) {
	if s.storage == nil {
		return
	}
	backend := s.storage.GetClient(ctx)
	if backend == nil {
		return
	}

	var files []models.SkillFile
	err := s.db.NewSelect().Model(&files).Where("skill_id = ?", srcSkillID).Scan(ctx)
	if err != nil || len(files) == 0 {
		return
	}

	for _, f := range files {
		newS3Key := fmt.Sprintf("skills/%d/%s", dstSkillID, f.Filepath)
		if err := backend.Copy(ctx, f.S3Key, newS3Key); err != nil {
			log.Warn().Err(err).Str("filepath", f.Filepath).Msg("failed to copy S3 file for fork")
			continue
		}
		newFile := &models.SkillFile{
			SkillID:     dstSkillID,
			Filepath:    f.Filepath,
			S3Key:       newS3Key,
			Checksum:    f.Checksum,
			Size:        f.Size,
			ContentType: f.ContentType,
			CreatedAt:   time.Now(),
		}
		if _, err := s.db.NewInsert().Model(newFile).Exec(ctx); err != nil {
			log.Warn().Err(err).Str("filepath", f.Filepath).Msg("failed to insert forked file record")
		}
	}

	// Recompute content_checksum for the destination (forked) skill
	if err := s.RebuildSkillBundle(ctx, dstSkillID); err != nil {
		log.Warn().Err(err).Int64("skill_id", dstSkillID).Msg("failed to recompute content_checksum after fork")
	}
}

// SanitizeCommandName converts a skill name to a kebab-case command name.
// "Revenue Analysis Report" → "revenue-analysis-report"
func SanitizeCommandName(name string) string {
	result := strings.ToLower(name)

	var cleaned strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	result = cleaned.String()

	result = strings.Join(strings.Fields(result), "-")
	result = strings.Trim(result, "-")

	return result
}
