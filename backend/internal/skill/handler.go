package skill

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db          *bun.DB
	syncService *SyncService
}

func NewHandler(db *bun.DB, containerManager *container.Manager) *Handler {
	return &Handler{
		db:          db,
		syncService: NewSyncService(db, containerManager),
	}
}

// GetSyncService returns the handler's SyncService for use by other handlers.
func (h *Handler) GetSyncService() *SyncService {
	return h.syncService
}

// CreateSkill creates a new skill
func (h *Handler) CreateSkill(c *gin.Context) {
	var skill models.Skill
	if err := c.ShouldBindJSON(&skill); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	// Get user ID from context (should be set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	skill.CreatedBy = userID.(int64)
	skill.CreatedAt = time.Now()
	skill.UpdatedAt = time.Now()
	skill.IsOfficial = false // Only admins can create official skills

	// Auto-generate command_name from name if not provided
	if skill.CommandName == "" {
		skill.CommandName = SanitizeCommandName(skill.Name)
	}

	if skill.CommandName == "" {
		response.BadRequest(c, "Cannot derive a valid command name from skill name")
		return
	}

	ctx := context.Background()

	// Check command_name uniqueness
	exists, err := h.db.NewSelect().Model((*models.Skill)(nil)).
		Where("command_name = ?", skill.CommandName).
		Exists(ctx)
	if err != nil {
		response.InternalError(c, "Failed to check command name", err)
		return
	}
	if exists {
		response.Conflict(c, fmt.Sprintf("Command name '/%s' is already taken", skill.CommandName))
		return
	}

	_, err = h.db.NewInsert().Model(&skill).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create skill", err)
		return
	}

	response.Created(c, skill)
}

// GetSkills retrieves all skills for the current user
func (h *Handler) GetSkills(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	ctx := context.Background()
	var skills []models.Skill

	// Get official skills + user's own skills + public skills
	err := h.db.NewSelect().
		Model(&skills).
		Where("is_official = ? OR created_by = ? OR is_public = ?", true, userID.(int64), true).
		Order("category ASC", "name ASC").
		Scan(ctx)

	if err != nil {
		response.InternalError(c, "Failed to retrieve skills", err)
		return
	}

	response.OK(c, skills)
}

// GetSkill retrieves a single skill by ID
func (h *Handler) GetSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	ctx := context.Background()
	var skill models.Skill

	err = h.db.NewSelect().
		Model(&skill).
		Where("id = ?", skillID).
		Scan(ctx)

	if err != nil {
		response.NotFound(c, "Skill not found", err)
		return
	}

	response.OK(c, skill)
}

// UpdateSkill updates an existing skill
func (h *Handler) UpdateSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	ctx := context.Background()

	// Check ownership
	var existingSkill models.Skill
	err = h.db.NewSelect().
		Model(&existingSkill).
		Where("id = ?", skillID).
		Scan(ctx)

	if err != nil {
		response.NotFound(c, "Skill not found", err)
		return
	}

	if existingSkill.CreatedBy != userID.(int64) && !existingSkill.IsOfficial {
		response.Forbidden(c, "You don't have permission to update this skill")
		return
	}

	// Parse update data
	var updateData models.Skill
	if err := c.ShouldBindJSON(&updateData); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	updateData.ID = skillID
	updateData.UpdatedAt = time.Now()

	// If user supplied a command_name, use it; otherwise regenerate from name
	if updateData.CommandName == "" && updateData.Name != "" {
		updateData.CommandName = SanitizeCommandName(updateData.Name)
	}

	if updateData.CommandName == "" {
		updateData.CommandName = existingSkill.CommandName
	}

	// Check command_name uniqueness (exclude self)
	if updateData.CommandName != existingSkill.CommandName {
		dup, dupErr := h.db.NewSelect().Model((*models.Skill)(nil)).
			Where("command_name = ? AND id != ?", updateData.CommandName, skillID).
			Exists(ctx)
		if dupErr != nil {
			response.InternalError(c, "Failed to check command name", dupErr)
			return
		}
		if dup {
			response.Conflict(c, fmt.Sprintf("Command name '/%s' is already taken", updateData.CommandName))
			return
		}
	}

	_, err = h.db.NewUpdate().
		Model(&updateData).
		Column("name", "description", "icon", "category", "prompt", "command_name", "parameters", "is_public", "updated_at").
		Where("id = ?", skillID).
		Exec(ctx)

	if err != nil {
		response.InternalError(c, "Failed to update skill", err)
		return
	}

	// Re-read full record from DB to return complete data
	var updatedSkill models.Skill
	err = h.db.NewSelect().
		Model(&updatedSkill).
		Where("id = ?", skillID).
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to reload skill after update", err)
		return
	}

	// Async: sync updated skill to all agents that have it installed
	go func() {
		bgCtx := context.Background()
		var agentSkills []models.AgentSkill
		_ = h.db.NewSelect().
			Model(&agentSkills).
			Where("skill_id = ?", skillID).
			Scan(bgCtx)

		for _, as := range agentSkills {
			// Look up the agent to get user ID
			var agent models.Agent
			err := h.db.NewSelect().Model(&agent).Where("id = ?", as.AgentID).Scan(bgCtx)
			if err != nil {
				continue
			}
			userIDStr := fmt.Sprintf("%d", agent.CreatedBy)

			// If name changed, remove old command file
			if existingSkill.CommandName != "" && existingSkill.CommandName != updatedSkill.CommandName {
				_ = h.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, as.AgentID, existingSkill.CommandName)
			}
			// Write updated file
			_ = h.syncService.SyncSkillToAgent(bgCtx, userIDStr, as.AgentID, &updatedSkill)
		}
	}()

	response.OK(c, updatedSkill)
}

// DeleteSkill deletes a skill
func (h *Handler) DeleteSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	ctx := context.Background()

	// Check ownership
	var skill models.Skill
	err = h.db.NewSelect().
		Model(&skill).
		Where("id = ?", skillID).
		Scan(ctx)

	if err != nil {
		response.NotFound(c, "Skill not found", err)
		return
	}

	if skill.CreatedBy != userID.(int64) {
		response.Forbidden(c, "You don't have permission to delete this skill")
		return
	}

	if skill.IsOfficial {
		response.Forbidden(c, "Cannot delete official skills")
		return
	}

	// Find all agents that have this skill installed (before deleting)
	var agentSkills []models.AgentSkill
	_ = h.db.NewSelect().
		Model(&agentSkills).
		Where("skill_id = ?", skillID).
		Scan(ctx)

	_, err = h.db.NewDelete().
		Model(&skill).
		Where("id = ?", skillID).
		Exec(ctx)

	if err != nil {
		response.InternalError(c, "Failed to delete skill", err)
		return
	}

	// Async: remove .md files from all agent pods
	if skill.CommandName != "" {
		go func() {
			bgCtx := context.Background()
			for _, as := range agentSkills {
				var agent models.Agent
				err := h.db.NewSelect().Model(&agent).Where("id = ?", as.AgentID).Scan(bgCtx)
				if err != nil {
					continue
				}
				userIDStr := fmt.Sprintf("%d", agent.CreatedBy)
				if err := h.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, as.AgentID, skill.CommandName); err != nil {
					log.Printf("Warning: failed to remove skill /%s from agent %d: %v", skill.CommandName, as.AgentID, err)
				}
			}
		}()
	}

	response.Success(c, "Skill deleted successfully")
}

// ForkSkill creates a copy of a public skill
func (h *Handler) ForkSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	ctx := context.Background()

	// Get original skill
	var originalSkill models.Skill
	err = h.db.NewSelect().
		Model(&originalSkill).
		Where("id = ?", skillID).
		Scan(ctx)

	if err != nil {
		response.NotFound(c, "Skill not found", err)
		return
	}

	if !originalSkill.IsPublic && !originalSkill.IsOfficial {
		response.Forbidden(c, "This skill is not public")
		return
	}

	// Create forked skill with a unique command_name
	forkedName := originalSkill.Name + " (Fork)"
	baseCmd := SanitizeCommandName(forkedName)
	cmdName := baseCmd

	// Ensure uniqueness by appending a numeric suffix if needed
	for i := 2; i <= 100; i++ {
		exists, exErr := h.db.NewSelect().Model((*models.Skill)(nil)).
			Where("command_name = ?", cmdName).
			Exists(ctx)
		if exErr != nil {
			response.InternalError(c, "Failed to check command name", exErr)
			return
		}
		if !exists {
			break
		}
		cmdName = fmt.Sprintf("%s-%d", baseCmd, i)
	}

	forkedSkill := models.Skill{
		Name:        forkedName,
		Description: originalSkill.Description,
		Icon:        originalSkill.Icon,
		Category:    originalSkill.Category,
		Prompt:      originalSkill.Prompt,
		CommandName: cmdName,
		Parameters:  originalSkill.Parameters,
		IsOfficial:  false,
		CreatedBy:   userID.(int64),
		IsPublic:    false,
		ForkedFrom:  &originalSkill.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = h.db.NewInsert().Model(&forkedSkill).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to fork skill", err)
		return
	}

	response.Created(c, forkedSkill)
}

// GetPublicSkills retrieves all public skills
func (h *Handler) GetPublicSkills(c *gin.Context) {
	ctx := context.Background()
	var skills []models.Skill

	err := h.db.NewSelect().
		Model(&skills).
		Where("is_public = ? OR is_official = ?", true, true).
		Relation("Creator").
		Order("category ASC", "name ASC").
		Scan(ctx)

	if err != nil {
		response.InternalError(c, "Failed to retrieve public skills", err)
		return
	}

	response.OK(c, skills)
}

// RegisterRoutes registers skill routes
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/skills", h.GetSkills)
	router.GET("/skills/:id", h.GetSkill)
	router.POST("/skills", h.CreateSkill)
	router.PUT("/skills/:id", h.UpdateSkill)
	router.DELETE("/skills/:id", h.DeleteSkill)
	router.POST("/skills/:id/fork", h.ForkSkill)
	router.GET("/skills/public", h.GetPublicSkills)
}
