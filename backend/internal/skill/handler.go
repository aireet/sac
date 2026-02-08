package skill

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context (should be set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot derive a valid command name from skill name"})
		return
	}

	ctx := context.Background()

	// Check command_name uniqueness
	exists, err := h.db.NewSelect().Model((*models.Skill)(nil)).
		Where("command_name = ?", skill.CommandName).
		Exists(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check command name"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Command name '/%s' is already taken", skill.CommandName)})
		return
	}

	_, err = h.db.NewInsert().Model(&skill).Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create skill"})
		return
	}

	c.JSON(http.StatusCreated, skill)
}

// GetSkills retrieves all skills for the current user
func (h *Handler) GetSkills(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve skills"})
		return
	}

	c.JSON(http.StatusOK, skills)
}

// GetSkill retrieves a single skill by ID
func (h *Handler) GetSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid skill ID"})
		return
	}

	ctx := context.Background()
	var skill models.Skill

	err = h.db.NewSelect().
		Model(&skill).
		Where("id = ?", skillID).
		Scan(ctx)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	c.JSON(http.StatusOK, skill)
}

// UpdateSkill updates an existing skill
func (h *Handler) UpdateSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid skill ID"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	if existingSkill.CreatedBy != userID.(int64) && !existingSkill.IsOfficial {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to update this skill"})
		return
	}

	// Parse update data
	var updateData models.Skill
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check command name"})
			return
		}
		if dup {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Command name '/%s' is already taken", updateData.CommandName)})
			return
		}
	}

	_, err = h.db.NewUpdate().
		Model(&updateData).
		Column("name", "description", "icon", "category", "prompt", "command_name", "parameters", "is_public", "updated_at").
		Where("id = ?", skillID).
		Exec(ctx)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update skill"})
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
			if existingSkill.CommandName != "" && existingSkill.CommandName != updateData.CommandName {
				_ = h.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, as.AgentID, existingSkill.CommandName)
			}
			// Write updated file
			_ = h.syncService.SyncSkillToAgent(bgCtx, userIDStr, as.AgentID, &updateData)
		}
	}()

	c.JSON(http.StatusOK, updateData)
}

// DeleteSkill deletes a skill
func (h *Handler) DeleteSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid skill ID"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	if skill.CreatedBy != userID.(int64) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to delete this skill"})
		return
	}

	if skill.IsOfficial {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete official skills"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete skill"})
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

	c.JSON(http.StatusOK, gin.H{"message": "Skill deleted successfully"})
}

// ForkSkill creates a copy of a public skill
func (h *Handler) ForkSkill(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid skill ID"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	if !originalSkill.IsPublic && !originalSkill.IsOfficial {
		c.JSON(http.StatusForbidden, gin.H{"error": "This skill is not public"})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check command name"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fork skill"})
		return
	}

	c.JSON(http.StatusCreated, forkedSkill)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve public skills"})
		return
	}

	c.JSON(http.StatusOK, skills)
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
