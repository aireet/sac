package admin

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
	db               *bun.DB
	containerManager *container.Manager
}

func NewHandler(db *bun.DB, cm *container.Manager) *Handler {
	return &Handler{db: db, containerManager: cm}
}

func (h *Handler) GetSettings(c *gin.Context) {
	ctx := context.Background()
	var settings []models.SystemSetting
	err := h.db.NewSelect().Model(&settings).Order("key ASC").Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to fetch settings", err)
		return
	}
	response.OK(c, settings)
}

func (h *Handler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Value       models.SettingValue `json:"value" binding:"required"`
		Description *string             `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()

	q := h.db.NewUpdate().Model((*models.SystemSetting)(nil)).
		Set("value = ?", req.Value).
		Set("updated_at = ?", time.Now()).
		Where("key = ?", key)

	if req.Description != nil {
		q = q.Set("description = ?", *req.Description)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update setting", err)
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		response.NotFound(c, "Setting not found")
		return
	}

	response.Success(c, "Setting updated")
}

func (h *Handler) GetUsers(c *gin.Context) {
	ctx := context.Background()

	type UserWithCount struct {
		models.User
		AgentCount int `bun:"agent_count" json:"agent_count"`
	}

	var users []UserWithCount
	err := h.db.NewSelect().
		TableExpr("users AS u").
		ColumnExpr("u.*").
		ColumnExpr("(SELECT COUNT(*) FROM agents WHERE created_by = u.id) AS agent_count").
		Order("u.id ASC").
		Scan(ctx, &users)
	if err != nil {
		response.InternalError(c, "Failed to fetch users", err)
		return
	}

	response.OK(c, users)
}

func (h *Handler) UpdateUserRole(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}

	var req struct {
		Role string `json:"role" binding:"required,oneof=user admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	res, err := h.db.NewUpdate().Model((*models.User)(nil)).
		Set("role = ?", req.Role).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update role", err)
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		response.NotFound(c, "User not found")
		return
	}

	response.Success(c, "Role updated")
}

func (h *Handler) GetUserSettings(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}

	ctx := context.Background()
	var settings []models.UserSetting
	err = h.db.NewSelect().Model(&settings).
		Where("user_id = ?", userID).
		Order("key ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to fetch user settings", err)
		return
	}

	if settings == nil {
		settings = []models.UserSetting{}
	}
	response.OK(c, settings)
}

func (h *Handler) SetUserSetting(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}
	key := c.Param("key")

	var req struct {
		Value models.SettingValue `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	setting := &models.UserSetting{
		UserID:    userID,
		Key:       key,
		Value:     req.Value,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = h.db.NewInsert().Model(setting).
		On("CONFLICT (user_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to set user setting", err)
		return
	}

	response.Success(c, "User setting updated")
}

func (h *Handler) DeleteUserSetting(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}
	key := c.Param("key")

	ctx := context.Background()
	_, err = h.db.NewDelete().Model((*models.UserSetting)(nil)).
		Where("user_id = ? AND key = ?", userID, key).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to delete user setting", err)
		return
	}

	response.Success(c, "User setting deleted")
}

func (h *Handler) GetUserAgents(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}

	ctx := context.Background()
	userIDStr := fmt.Sprintf("%d", userID)

	var agents []models.Agent
	err = h.db.NewSelect().
		Model(&agents).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Where("ag.created_by = ?", userID).
		Order("ag.id ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to fetch agents", err)
		return
	}

	type agentWithStatus struct {
		models.Agent
		PodStatus     string `json:"pod_status"`
		RestartCount  int32  `json:"restart_count"`
		CPURequest    string `json:"cpu_request"`
		CPULimit      string `json:"cpu_limit"`
		MemoryRequest string `json:"memory_request"`
		MemoryLimit   string `json:"memory_limit"`
	}

	result := make([]agentWithStatus, 0, len(agents))
	for _, a := range agents {
		info := h.containerManager.GetStatefulSetPodInfo(ctx, userIDStr, a.ID)
		result = append(result, agentWithStatus{
			Agent:         a,
			PodStatus:     info.Status,
			RestartCount:  info.RestartCount,
			CPURequest:    info.CPURequest,
			CPULimit:      info.CPULimit,
			MemoryRequest: info.MemoryRequest,
			MemoryLimit:   info.MemoryLimit,
		})
	}

	response.OK(c, result)
}

func (h *Handler) DeleteUserAgent(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}
	agentID, err := strconv.ParseInt(c.Param("agentId"), 10, 64)
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
	}

	response.Success(c, "Agent deleted successfully")
}

func (h *Handler) RestartUserAgent(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}
	agentID, err := strconv.ParseInt(c.Param("agentId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	ctx := context.Background()
	userIDStr := fmt.Sprintf("%d", userID)

	// Verify agent exists and belongs to this user
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(ctx)
	if err != nil {
		response.NotFound(c, "Agent not found", err)
		return
	}

	// Mark active sessions as deleted
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

	// Delete the pod (K8s will recreate it)
	podName := fmt.Sprintf("claude-code-%s-%d-0", userIDStr, agentID)
	err = h.containerManager.DeletePodByName(ctx, podName)
	if err != nil {
		log.Printf("Failed to delete pod %s: %v", podName, err)
		response.InternalError(c, "Failed to restart agent pod", err)
		return
	}

	log.Printf("Admin restarted agent %d pod %s for user %d", agentID, podName, userID)
	response.Success(c, "Agent is restarting")
}

func (h *Handler) UpdateAgentResources(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", err)
		return
	}
	agentID, err := strconv.ParseInt(c.Param("agentId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid agent ID", err)
		return
	}

	var req struct {
		CPURequest    *string `json:"cpu_request"`
		CPULimit      *string `json:"cpu_limit"`
		MemoryRequest *string `json:"memory_request"`
		MemoryLimit   *string `json:"memory_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()

	// Verify agent exists and belongs to this user
	var agent models.Agent
	err = h.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(ctx)
	if err != nil {
		response.NotFound(c, "Agent not found")
		return
	}

	q := h.db.NewUpdate().Model((*models.Agent)(nil)).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", agentID)

	if req.CPURequest != nil {
		if *req.CPURequest == "" {
			q = q.Set("cpu_request = NULL")
		} else {
			q = q.Set("cpu_request = ?", *req.CPURequest)
		}
	}
	if req.CPULimit != nil {
		if *req.CPULimit == "" {
			q = q.Set("cpu_limit = NULL")
		} else {
			q = q.Set("cpu_limit = ?", *req.CPULimit)
		}
	}
	if req.MemoryRequest != nil {
		if *req.MemoryRequest == "" {
			q = q.Set("memory_request = NULL")
		} else {
			q = q.Set("memory_request = ?", *req.MemoryRequest)
		}
	}
	if req.MemoryLimit != nil {
		if *req.MemoryLimit == "" {
			q = q.Set("memory_limit = NULL")
		} else {
			q = q.Set("memory_limit = ?", *req.MemoryLimit)
		}
	}

	_, err = q.Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update agent resources", err)
		return
	}

	response.Success(c, "Agent resources updated. Restart agent to apply.")
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	admin := rg.Group("/admin")
	admin.Use(AdminMiddleware())
	{
		admin.GET("/settings", h.GetSettings)
		admin.PUT("/settings/:key", h.UpdateSetting)
		admin.GET("/users", h.GetUsers)
		admin.PUT("/users/:id/role", h.UpdateUserRole)
		admin.GET("/users/:id/settings", h.GetUserSettings)
		admin.PUT("/users/:id/settings/:key", h.SetUserSetting)
		admin.DELETE("/users/:id/settings/:key", h.DeleteUserSetting)
		admin.GET("/users/:id/agents", h.GetUserAgents)
		admin.DELETE("/users/:id/agents/:agentId", h.DeleteUserAgent)
		admin.POST("/users/:id/agents/:agentId/restart", h.RestartUserAgent)
		admin.PUT("/users/:id/agents/:agentId/resources", h.UpdateAgentResources)
	}
}
