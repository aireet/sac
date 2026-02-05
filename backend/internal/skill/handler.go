package skill

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/echotech/sac/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) *Handler {
	return &Handler{db: db}
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

	ctx := context.Background()
	_, err := h.db.NewInsert().Model(&skill).Exec(ctx)
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

	_, err = h.db.NewUpdate().
		Model(&updateData).
		Column("name", "description", "icon", "category", "prompt", "parameters", "is_public", "updated_at").
		Where("id = ?", skillID).
		Exec(ctx)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update skill"})
		return
	}

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

	_, err = h.db.NewDelete().
		Model(&skill).
		Where("id = ?", skillID).
		Exec(ctx)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete skill"})
		return
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

	// Create forked skill
	forkedSkill := models.Skill{
		Name:        originalSkill.Name + " (Fork)",
		Description: originalSkill.Description,
		Icon:        originalSkill.Icon,
		Category:    originalSkill.Category,
		Prompt:      originalSkill.Prompt,
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
