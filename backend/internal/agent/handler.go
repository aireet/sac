package agent

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

const MaxAgentsPerUser = 3

type Handler struct {
	db               *bun.DB
	containerManager *container.Manager
	syncService      *skill.SyncService
}

func NewHandler(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService) *Handler {
	return &Handler{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
	}
}

// RegisterRoutes registers agent routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	// Register /agent-statuses at the group level to avoid conflict with /agents/:id
	rg.GET("/agent-statuses", h.GetAgentStatuses)

	agents := rg.Group("/agents")
	{
		agents.GET("", h.GetAgents)
		agents.GET("/:id", h.GetAgent)
		agents.POST("", h.CreateAgent)
		agents.PUT("/:id", h.UpdateAgent)
		agents.DELETE("/:id", h.DeleteAgent)
		agents.POST("/:id/restart", h.RestartAgent)
		agents.POST("/:id/skills", h.InstallSkill)
		agents.DELETE("/:id/skills/:skillId", h.UninstallSkill)
		agents.POST("/:id/sync-skills", h.SyncSkills)
	}
}

// GetAgents returns all agents for the current user
func (h *Handler) GetAgents(c *gin.Context) {
	userID := c.GetInt64("userID")

	agents := make([]models.Agent, 0)
	err := h.db.NewSelect().
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

	if agents == nil {
		agents = []models.Agent{}
	}
	c.JSON(http.StatusOK, agents)
}

// GetAgent returns a specific agent by ID
func (h *Handler) GetAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	var agent models.Agent
	err = h.db.NewSelect().
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
func (h *Handler) CreateAgent(c *gin.Context) {
	userID := c.GetInt64("userID")

	// Check if user already has max agents
	count, err := h.db.NewSelect().
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

	_, err = h.db.NewInsert().
		Model(agent).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create agent"})
		return
	}

	c.JSON(http.StatusCreated, agent)
}

// UpdateAgent updates an existing agent
func (h *Handler) UpdateAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
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
	err = h.db.NewSelect().
		Model(&existing).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Update fields
	_, err = h.db.NewUpdate().
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

	// If config changed, delete existing StatefulSet so it gets recreated with new env vars
	if req.Config != nil {
		ctx := context.Background()
		userIDStr := fmt.Sprintf("%d", userID)

		// Mark related sessions as deleted
		_, _ = h.db.NewUpdate().
			Model((*models.Session)(nil)).
			Set("status = ?", models.SessionStatusDeleted).
			Set("updated_at = ?", time.Now()).
			Where("agent_id = ?", agentID).
			Where("user_id = ?", userID).
			Where("status IN (?)", bun.In([]string{string(models.SessionStatusRunning), string(models.SessionStatusCreating), string(models.SessionStatusIdle)})).
			Exec(ctx)

		// Delete old StatefulSet (ignore errors if it doesn't exist)
		if err := h.containerManager.DeleteStatefulSet(ctx, userIDStr, agentID); err != nil {
			log.Printf("Note: no existing StatefulSet to delete for agent %d: %v", agentID, err)
		}
	}

	// Fetch updated agent
	err = h.db.NewSelect().
		Model(&existing).
		Where("id = ?", agentID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated agent"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeleteAgent deletes an agent and cleans up its K8s StatefulSet
func (h *Handler) DeleteAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	ctx := context.Background()
	userIDStr := fmt.Sprintf("%d", userID)

	// Delete agent from DB (cascade will delete agent_skills)
	res, err := h.db.NewDelete().
		Model((*models.Agent)(nil)).
		Where("id = ? AND created_by = ?", agentID, userID).
		Exec(ctx)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete agent"})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Mark related sessions as deleted
	_, err = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", agentID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		log.Printf("Warning: failed to clean up sessions for agent %d: %v", agentID, err)
	}

	// Delete K8s StatefulSet and headless service
	if err := h.containerManager.DeleteStatefulSet(ctx, userIDStr, agentID); err != nil {
		log.Printf("Warning: failed to delete StatefulSet for agent %d: %v", agentID, err)
		// Not fatal — DB record is already deleted
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted successfully"})
}

// RestartAgent deletes the StatefulSet pod so K8s recreates it
func (h *Handler) RestartAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	ctx := context.Background()
	userIDStr := fmt.Sprintf("%d", userID)

	// Mark existing sessions as deleted
	_, _ = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", agentID).
		Where("user_id = ?", userID).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	// Delete the StatefulSet pod (not the StatefulSet itself)
	// K8s will automatically recreate the pod
	podName := fmt.Sprintf("claude-code-%s-%d-0", userIDStr, agentID)
	err = h.containerManager.DeletePodByName(ctx, podName)
	if err != nil {
		log.Printf("Failed to delete pod %s: %v", podName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restart agent pod"})
		return
	}

	log.Printf("Restarted agent %d pod %s for user %s", agentID, podName, userIDStr)
	c.JSON(http.StatusOK, gin.H{"message": "Agent is restarting"})
}

// InstallSkill installs a skill to an agent
func (h *Handler) InstallSkill(c *gin.Context) {
	userID := c.GetInt64("userID")
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
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Verify skill exists
	var skill models.Skill
	err = h.db.NewSelect().
		Model(&skill).
		Where("id = ?", req.SkillID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Skill not found"})
		return
	}

	// Get current max order
	var maxOrder int
	err = h.db.NewSelect().
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

	_, err = h.db.NewInsert().
		Model(agentSkill).
		On("CONFLICT (agent_id, skill_id) DO NOTHING").
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to install skill"})
		return
	}

	// Async: sync skill file to pod
	go func() {
		bgCtx := context.Background()
		userIDStr := fmt.Sprintf("%d", userID)
		if err := h.syncService.SyncSkillToAgent(bgCtx, userIDStr, agentID, &skill); err != nil {
			log.Printf("Warning: failed to sync skill /%s to agent %d: %v", skill.CommandName, agentID, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Skill installed successfully"})
}

// GetAgentStatuses returns the K8s pod status for all agents of the current user
func (h *Handler) GetAgentStatuses(c *gin.Context) {
	userID := c.GetInt64("userID")
	userIDStr := fmt.Sprintf("%d", userID)

	// Get all agent IDs for this user
	var agentIDs []int64
	err := h.db.NewSelect().
		Model((*models.Agent)(nil)).
		Column("id").
		Where("created_by = ?", userID).
		Scan(c.Request.Context(), &agentIDs)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents"})
		return
	}

	type agentStatus struct {
		AgentID       int64  `json:"agent_id"`
		Status        string `json:"status"`
		RestartCount  int32  `json:"restart_count"`
		CPURequest    string `json:"cpu_request"`
		CPULimit      string `json:"cpu_limit"`
		MemoryRequest string `json:"memory_request"`
		MemoryLimit   string `json:"memory_limit"`
	}

	statuses := make([]agentStatus, 0, len(agentIDs))
	for _, aid := range agentIDs {
		info := h.containerManager.GetStatefulSetPodInfo(c.Request.Context(), userIDStr, aid)
		statuses = append(statuses, agentStatus{
			AgentID:       aid,
			Status:        info.Status,
			RestartCount:  info.RestartCount,
			CPURequest:    info.CPURequest,
			CPULimit:      info.CPULimit,
			MemoryRequest: info.MemoryRequest,
			MemoryLimit:   info.MemoryLimit,
		})
	}

	c.JSON(http.StatusOK, statuses)
}

// UninstallSkill removes a skill from an agent
func (h *Handler) UninstallSkill(c *gin.Context) {
	userID := c.GetInt64("userID")
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
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Look up the skill to get command_name before uninstalling
	var sk models.Skill
	_ = h.db.NewSelect().Model(&sk).Where("id = ?", skillID).Scan(c.Request.Context())

	// Uninstall skill
	_, err = h.db.NewDelete().
		Model((*models.AgentSkill)(nil)).
		Where("agent_id = ? AND skill_id = ?", agentID, skillID).
		Exec(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to uninstall skill"})
		return
	}

	// Async: remove skill file from pod
	if sk.CommandName != "" {
		go func() {
			bgCtx := context.Background()
			userIDStr := fmt.Sprintf("%d", userID)
			if err := h.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, agentID, sk.CommandName); err != nil {
				log.Printf("Warning: failed to remove skill /%s from agent %d: %v", sk.CommandName, agentID, err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Skill uninstalled successfully"})
}

// SyncSkills manually triggers a full sync of all installed skills to the agent pod.
func (h *Handler) SyncSkills(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	userIDStr := fmt.Sprintf("%d", userID)
	if err := h.syncService.SyncAllSkillsToAgent(c.Request.Context(), userIDStr, agentID); err != nil {
		log.Printf("Failed to sync skills for agent %d: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync skills"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Skills synced successfully"})
}
