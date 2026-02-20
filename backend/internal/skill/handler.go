package skill

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/protobind"
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
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	req := &sacv1.CreateSkillRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Name == "" {
		response.BadRequest(c, "name is required")
		return
	}

	skill := models.Skill{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Category:    req.Category,
		Prompt:      req.Prompt,
		CommandName: req.CommandName,
		Parameters:  convert.SkillParametersFromProto(req.Parameters),
		IsPublic:    req.IsPublic,
		CreatedBy:   userID.(int64),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsOfficial:  false,
	}

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
	exists2, err := h.db.NewSelect().Model((*models.Skill)(nil)).
		Where("command_name = ?", skill.CommandName).
		Exists(ctx)
	if err != nil {
		response.InternalError(c, "Failed to check command name", err)
		return
	}
	if exists2 {
		response.Conflict(c, fmt.Sprintf("Command name '/%s' is already taken", skill.CommandName))
		return
	}

	_, err = h.db.NewInsert().Model(&skill).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create skill", err)
		return
	}

	protobind.Created(c, convert.SkillToProto(&skill))
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

	protobind.OK(c, &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)})
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

	protobind.OK(c, convert.SkillToProto(&skill))
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

	// Ownership check: owner can edit their own skill; admin can edit official skills
	role, _ := c.Get("role")
	isAdmin := role == "admin"
	if existingSkill.IsOfficial && !isAdmin {
		response.Forbidden(c, "Only admins can edit official skills")
		return
	}
	if !existingSkill.IsOfficial && existingSkill.CreatedBy != userID.(int64) {
		response.Forbidden(c, "You don't have permission to update this skill")
		return
	}

	// Parse update data via protobuf
	req := &sacv1.UpdateSkillRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	var updateData models.Skill
	updateData.ID = skillID
	updateData.UpdatedAt = time.Now()
	updateData.Version = existingSkill.Version + 1

	// Apply optional fields from proto request
	if req.Name != nil {
		updateData.Name = *req.Name
	}
	if req.Description != nil {
		updateData.Description = *req.Description
	}
	if req.Icon != nil {
		updateData.Icon = *req.Icon
	}
	if req.Category != nil {
		updateData.Category = *req.Category
	}
	if req.Prompt != nil {
		updateData.Prompt = *req.Prompt
	}
	if req.CommandName != nil {
		updateData.CommandName = *req.CommandName
	}
	if req.Parameters != nil {
		updateData.Parameters = convert.SkillParametersFromProto(req.Parameters)
	}
	if req.IsPublic != nil {
		updateData.IsPublic = *req.IsPublic
	}

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
		Column("name", "description", "icon", "category", "prompt", "command_name", "parameters", "is_public", "updated_at", "version").
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

	protobind.OK(c, convert.SkillToProto(&updatedSkill))
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

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Skill deleted successfully"})
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
		exists2, exErr := h.db.NewSelect().Model((*models.Skill)(nil)).
			Where("command_name = ?", cmdName).
			Exists(ctx)
		if exErr != nil {
			response.InternalError(c, "Failed to check command name", exErr)
			return
		}
		if !exists2 {
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

	protobind.Created(c, convert.SkillToProto(&forkedSkill))
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

	protobind.OK(c, &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)})
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
