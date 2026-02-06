package agent

import (
	"net/http"
	"strconv"

	"github.com/echotech/sac/internal/database"
	"github.com/echotech/sac/internal/models"
	"github.com/gin-gonic/gin"
)

const MaxAgentsPerUser = 3

// GetAgents returns all agents for the current user
func GetAgents(c *gin.Context) {
	// TODO: Get user ID from auth context
	userID := int64(1) // Mock user ID

	var agents []models.Agent
	err := database.DB.NewSelect().
		Model(&agents).
		Where("created_by = ?", userID).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Order("created_at DESC").
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents"})
		return
	}

	c.JSON(http.StatusOK, agents)
}

// GetAgent returns a specific agent by ID
func GetAgent(c *gin.Context) {
	userID := int64(1) // Mock user ID
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	var agent models.Agent
	err = database.DB.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	c.JSON(http.StatusOK, agent)
}

// CreateAgent creates a new agent
func CreateAgent(c *gin.Context) {
	userID := int64(1) // Mock user ID

	// Check if user already has max agents
	count, err := database.DB.NewSelect().
		Model((*models.Agent)(nil)).
		Where("created_by = ?", userID).
		Count(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check agent count"})
		return
	}

	if count >= MaxAgentsPerUser {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Maximum agents limit reached",
			"message": "You can only create up to 3 agents",
		})
		return
	}

	var req struct {
		Name        string                 `json:"name" binding:"required"`
		Description string                 `json:"description"`
		Icon        string                 `json:"icon"`
		Config      map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agent := &models.Agent{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Config:      req.Config,
		CreatedBy:   userID,
	}

	_, err = database.DB.NewInsert().
		Model(agent).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create agent"})
		return
	}

	c.JSON(http.StatusCreated, agent)
}

// UpdateAgent updates an existing agent
func UpdateAgent(c *gin.Context) {
	userID := int64(1) // Mock user ID
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	var req struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Icon        string                 `json:"icon"`
		Config      map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify ownership
	var existing models.Agent
	err = database.DB.NewSelect().
		Model(&existing).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Update fields
	_, err = database.DB.NewUpdate().
		Model(&models.Agent{}).
		Set("name = ?", req.Name).
		Set("description = ?", req.Description).
		Set("icon = ?", req.Icon).
		Set("config = ?", req.Config).
		Where("id = ?", agentID).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update agent"})
		return
	}

	// Fetch updated agent
	err = database.DB.NewSelect().
		Model(&existing).
		Where("id = ?", agentID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated agent"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeleteAgent deletes an agent
func DeleteAgent(c *gin.Context) {
	userID := int64(1) // Mock user ID
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Delete agent (cascade will delete agent_skills)
	res, err := database.DB.NewDelete().
		Model((*models.Agent)(nil)).
		Where("id = ? AND created_by = ?", agentID, userID).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete agent"})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted successfully"})
}

// InstallSkill installs a skill to an agent
func InstallSkill(c *gin.Context) {
	userID := int64(1) // Mock user ID
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	var req struct {
		SkillID int64 `json:"skill_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = database.DB.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Verify skill exists
	var skill models.Skill
	err = database.DB.NewSelect().
		Model(&skill).
		Where("id = ?", req.SkillID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	// Get current max order
	var maxOrder int
	err = database.DB.NewSelect().
		Model((*models.AgentSkill)(nil)).
		Column("order").
		Where("agent_id = ?", agentID).
		Order("order DESC").
		Limit(1).
		Scan(c.Request.Context(), &maxOrder)

	// Install skill
	agentSkill := &models.AgentSkill{
		AgentID: agentID,
		SkillID: req.SkillID,
		Order:   maxOrder + 1,
	}

	_, err = database.DB.NewInsert().
		Model(agentSkill).
		On("CONFLICT (agent_id, skill_id) DO NOTHING").
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to install skill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Skill installed successfully"})
}

// UninstallSkill removes a skill from an agent
func UninstallSkill(c *gin.Context) {
	userID := int64(1) // Mock user ID
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	skillID, err := strconv.ParseInt(c.Param("skillId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid skill ID"})
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = database.DB.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Uninstall skill
	_, err = database.DB.NewDelete().
		Model((*models.AgentSkill)(nil)).
		Where("agent_id = ? AND skill_id = ?", agentID, skillID).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to uninstall skill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Skill uninstalled successfully"})
}
