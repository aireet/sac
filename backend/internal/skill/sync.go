package skill

import (
	"context"
	"fmt"
	"log"
	"strings"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

const commandsDir = "/root/.claude/commands"

// SyncService handles syncing skill .md files to agent pods.
type SyncService struct {
	db               *bun.DB
	containerManager *container.Manager
}

// NewSyncService creates a new SyncService.
func NewSyncService(db *bun.DB, containerManager *container.Manager) *SyncService {
	return &SyncService{
		db:               db,
		containerManager: containerManager,
	}
}

// podName returns the StatefulSet pod name for a user-agent pair.
func (s *SyncService) podName(userID string, agentID int64) string {
	return fmt.Sprintf("claude-code-%s-%d-0", userID, agentID)
}

// SyncSkillToAgent writes a single skill's .md file into the agent pod.
func (s *SyncService) SyncSkillToAgent(ctx context.Context, userID string, agentID int64, sk *models.Skill) error {
	if sk.CommandName == "" {
		return fmt.Errorf("skill %d has no command_name", sk.ID)
	}

	pod := s.podName(userID, agentID)
	filePath := fmt.Sprintf("%s/%s.md", commandsDir, sk.CommandName)

	if err := s.containerManager.WriteFileInPod(ctx, pod, filePath, sk.Prompt); err != nil {
		return fmt.Errorf("failed to sync skill %q to agent %d: %w", sk.CommandName, agentID, err)
	}

	// Update synced_version to mark this skill as up-to-date on this agent
	_, _ = s.db.NewUpdate().
		Model((*models.AgentSkill)(nil)).
		Set("synced_version = ?", sk.Version).
		Where("agent_id = ?", agentID).
		Where("skill_id = ?", sk.ID).
		Exec(ctx)

	log.Printf("Synced skill /%s (v%d) to pod %s", sk.CommandName, sk.Version, pod)
	return nil
}

// RemoveSkillFromAgent deletes a skill's .md file from the agent pod.
func (s *SyncService) RemoveSkillFromAgent(ctx context.Context, userID string, agentID int64, commandName string) error {
	if commandName == "" {
		return nil
	}

	pod := s.podName(userID, agentID)
	filePath := fmt.Sprintf("%s/%s.md", commandsDir, commandName)

	if err := s.containerManager.DeleteFileInPod(ctx, pod, filePath); err != nil {
		return fmt.Errorf("failed to remove skill /%s from agent %d: %w", commandName, agentID, err)
	}

	log.Printf("Removed skill /%s from pod %s", commandName, pod)
	return nil
}

// SyncAllSkillsToAgent syncs all installed skills for an agent to its pod.
// It also cleans up .md files for skills that are no longer installed.
func (s *SyncService) SyncAllSkillsToAgent(ctx context.Context, userID string, agentID int64) error {
	// Query all installed skills for this agent
	var skills []models.Skill
	err := s.db.NewSelect().
		Model(&skills).
		Join("JOIN agent_skills AS ags ON ags.skill_id = sk.id").
		Where("ags.agent_id = ?", agentID).
		Scan(ctx)

	if err != nil {
		return fmt.Errorf("failed to query skills for agent %d: %w", agentID, err)
	}

	pod := s.podName(userID, agentID)

	// Build set of expected command files
	expectedFiles := make(map[string]bool)

	// Write each skill's .md file
	for i := range skills {
		sk := &skills[i]
		if sk.CommandName == "" {
			continue
		}
		fileName := sk.CommandName + ".md"
		expectedFiles[fileName] = true

		filePath := fmt.Sprintf("%s/%s", commandsDir, fileName)
		if err := s.containerManager.WriteFileInPod(ctx, pod, filePath, sk.Prompt); err != nil {
			log.Printf("Warning: failed to sync skill /%s to pod %s: %v", sk.CommandName, pod, err)
			continue
		}

		// Update synced_version to mark this skill as up-to-date
		_, _ = s.db.NewUpdate().
			Model((*models.AgentSkill)(nil)).
			Set("synced_version = ?", sk.Version).
			Where("agent_id = ?", agentID).
			Where("skill_id = ?", sk.ID).
			Exec(ctx)
	}

	// Clean up: list existing files and remove any that shouldn't be there
	existingFiles, err := s.containerManager.ListFilesInPod(ctx, pod, commandsDir)
	if err != nil {
		log.Printf("Warning: failed to list commands dir in pod %s: %v", pod, err)
		return nil // Not fatal
	}

	for _, f := range existingFiles {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		if !expectedFiles[f] {
			filePath := fmt.Sprintf("%s/%s", commandsDir, f)
			if err := s.containerManager.DeleteFileInPod(ctx, pod, filePath); err != nil {
				log.Printf("Warning: failed to clean up %s from pod %s: %v", f, pod, err)
			} else {
				log.Printf("Cleaned up stale command file %s from pod %s", f, pod)
			}
		}
	}

	log.Printf("Synced %d skills to pod %s", len(skills), pod)
	return nil
}

// SanitizeCommandName converts a skill name to a kebab-case command name.
// "Revenue Analysis Report" → "revenue-analysis-report"
func SanitizeCommandName(name string) string {
	// Lowercase
	result := strings.ToLower(name)

	// Replace non-alphanumeric characters (except spaces and hyphens) with empty string
	var cleaned strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	result = cleaned.String()

	// Replace spaces with hyphens
	result = strings.Join(strings.Fields(result), "-")

	// Remove leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
