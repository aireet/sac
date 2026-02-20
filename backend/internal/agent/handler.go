package agent

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db               *bun.DB
	containerManager *container.Manager
	syncService      *skill.SyncService
	settingsService  *admin.SettingsService
}

func NewHandler(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService, settingsService *admin.SettingsService) *Handler {
	return &Handler{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
		settingsService:  settingsService,
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
		agents.GET("/:id/claude-md-preview", h.PreviewClaudeMD)
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
		response.InternalError(c, "Failed to fetch agents", err)
		return
	}

	if agents == nil {
		agents = []models.Agent{}
	}
	protobind.OK(c, &sacv1.AgentListResponse{Agents: convert.AgentsToProto(agents)})
}

// GetAgent returns a specific agent by ID
func (h *Handler) GetAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
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
		response.NotFound(c, "Agent not found", err)
		return
	}

	protobind.OK(c, convert.AgentToProto(&agent))
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
		response.InternalError(c, "Failed to check agent count", err)
		return
	}

	maxAgents, _ := h.settingsService.GetMaxAgents(c.Request.Context(), userID)
	if count >= maxAgents {
		response.BadRequest(c, fmt.Sprintf("Maximum agents limit reached, you can only create up to %d agents", maxAgents))
		return
	}

	req := &sacv1.CreateAgentRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Name == "" {
		response.BadRequest(c, "name is required")
		return
	}

	agent := &models.Agent{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Instructions: req.Instructions,
		CreatedBy:    userID,
	}
	if req.Config != nil {
		agent.Config = req.Config.AsMap()
	}

	_, err = h.db.NewInsert().
		Model(agent).
		Exec(c.Request.Context())

	if err != nil {
		response.InternalError(c, "Failed to create agent", err)
		return
	}

	protobind.JSON(c, http.StatusCreated, convert.AgentToProto(agent))
}

// UpdateAgent updates an existing agent
func (h *Handler) UpdateAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	req := &sacv1.UpdateAgentRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	// Verify ownership
	var existing models.Agent
	err = h.db.NewSelect().
		Model(&existing).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		response.NotFound(c, "Agent not found", err)
		return
	}

	// Only update fields that were provided
	q := h.db.NewUpdate().Model(&models.Agent{}).Where("id = ?", agentID)
	if req.Name != nil {
		q = q.Set("name = ?", *req.Name)
	}
	if req.Description != nil {
		q = q.Set("description = ?", *req.Description)
	}
	if req.Icon != nil {
		q = q.Set("icon = ?", *req.Icon)
	}
	if req.Instructions != nil {
		q = q.Set("instructions = ?", *req.Instructions)
	}
	if req.Config != nil {
		q = q.Set("config = ?", models.AgentConfig(req.Config.AsMap()))
	}
	q = q.Set("updated_at = ?", time.Now())

	_, err = q.Exec(c.Request.Context())

	if err != nil {
		response.InternalError(c, "Failed to update agent", err)
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
		response.InternalError(c, "Failed to fetch updated agent", err)
		return
	}

	protobind.OK(c, convert.AgentToProto(&existing))
}

// DeleteAgent deletes an agent and cleans up its K8s StatefulSet
func (h *Handler) DeleteAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
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
		response.InternalError(c, "Failed to delete agent", err)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		response.NotFound(c, "Agent not found")
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

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Agent deleted successfully"})
}

// RestartAgent deletes the StatefulSet pod so K8s recreates it
func (h *Handler) RestartAgent(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())
	if err != nil {
		response.NotFound(c, "Agent not found", err)
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

	// Delete the entire StatefulSet so it's recreated with latest settings
	// (resource limits, docker image, etc.) on next session creation
	if err := h.containerManager.DeleteStatefulSet(ctx, userIDStr, agentID); err != nil {
		log.Printf("Failed to delete StatefulSet for agent %d: %v", agentID, err)
		response.InternalError(c, "Failed to restart agent", err)
		return
	}

	log.Printf("Restarted agent %d (deleted StatefulSet) for user %s", agentID, userIDStr)
	protobind.OK(c, &sacv1.SuccessMessage{Message: "Agent is restarting"})
}

// InstallSkill installs a skill to an agent
func (h *Handler) InstallSkill(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	req := &sacv1.InstallSkillRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.SkillId == 0 {
		response.BadRequest(c, "skill_id is required")
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		response.NotFound(c, "Agent not found", err)
		return
	}

	// Verify skill exists
	var sk models.Skill
	err = h.db.NewSelect().
		Model(&sk).
		Where("id = ?", req.SkillId).
		Scan(c.Request.Context())

	if err != nil {
		response.NotFound(c, "Skill not found", err)
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
		SkillID: req.SkillId,
		Order:   maxOrder + 1,
	}

	_, err = h.db.NewInsert().
		Model(agentSkill).
		On("CONFLICT (agent_id, skill_id) DO NOTHING").
		Exec(c.Request.Context())

	if err != nil {
		response.InternalError(c, "Failed to install skill", err)
		return
	}

	// Async: sync skill file to pod
	go func() {
		bgCtx := context.Background()
		userIDStr := fmt.Sprintf("%d", userID)
		if err := h.syncService.SyncSkillToAgent(bgCtx, userIDStr, agentID, &sk); err != nil {
			log.Printf("Warning: failed to sync skill /%s to agent %d: %v", sk.CommandName, agentID, err)
		}
	}()

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Skill installed successfully"})
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
		response.InternalError(c, "Failed to fetch agents", err)
		return
	}

	statuses := make([]*sacv1.AgentStatus, 0, len(agentIDs))
	for _, aid := range agentIDs {
		info := h.containerManager.GetStatefulSetPodInfo(c.Request.Context(), userIDStr, aid)
		statuses = append(statuses, &sacv1.AgentStatus{
			AgentId:       aid,
			PodName:       info.PodName,
			Status:        info.Status,
			RestartCount:  info.RestartCount,
			CpuRequest:    info.CPURequest,
			CpuLimit:      info.CPULimit,
			MemoryRequest: info.MemoryRequest,
			MemoryLimit:   info.MemoryLimit,
		})
	}

	protobind.OK(c, &sacv1.AgentStatusListResponse{Statuses: statuses})
}

// UninstallSkill removes a skill from an agent
func (h *Handler) UninstallSkill(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	skillID, err := strconv.ParseInt(c.Param("skillId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		response.NotFound(c, "Agent not found", err)
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
		response.InternalError(c, "Failed to uninstall skill", err)
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

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Skill uninstalled successfully"})
}

// SyncSkills manually triggers a full sync of all installed skills to the agent pod.
func (h *Handler) SyncSkills(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	// Verify agent ownership
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())

	if err != nil {
		response.NotFound(c, "Agent not found", err)
		return
	}

	userIDStr := fmt.Sprintf("%d", userID)
	if err := h.syncService.SyncAllSkillsToAgent(c.Request.Context(), userIDStr, agentID); err != nil {
		log.Printf("Failed to sync skills for agent %d: %v", agentID, err)
		response.InternalError(c, "Failed to sync skills", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Skills synced successfully"})
}

// PreviewClaudeMD returns the CLAUDE.md content split into read-only and editable parts.
func (h *Handler) PreviewClaudeMD(c *gin.Context) {
	userID := c.GetInt64("userID")
	agentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID")
		return
	}

	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(c.Request.Context())
	if err != nil {
		response.NotFound(c, "Agent not found", err)
		return
	}

	sysInstructions := h.settingsService.GetAgentSystemInstructions(c.Request.Context())
	groupTemplates := h.getGroupTemplates(c.Request.Context(), userID)

	var readonlyParts []string
	if sysInstructions != "" {
		readonlyParts = append(readonlyParts, sysInstructions)
	}
	readonlyParts = append(readonlyParts, groupTemplates...)

	protobind.OK(c, &sacv1.ClaudeMDPreview{
		Readonly:     strings.Join(readonlyParts, "\n\n---\n\n"),
		Instructions: agent.Instructions,
	})
}

func (h *Handler) getGroupTemplates(ctx context.Context, userID int64) []string {
	var templates []string
	err := h.db.NewSelect().
		TableExpr("groups AS g").
		ColumnExpr("g.claude_md_template").
		Join("JOIN group_members AS gm ON gm.group_id = g.id").
		Where("gm.user_id = ?", userID).
		Where("g.claude_md_template != ''").
		OrderExpr("g.name ASC").
		Scan(ctx, &templates)
	if err != nil {
		log.Printf("Warning: failed to get group templates for user %d: %v", userID, err)
		return nil
	}
	return templates
}
