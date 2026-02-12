package admin

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"strconv"
	"time"

	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	type GroupBrief struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}

	type UserWithCount struct {
		models.User
		AgentCount int          `bun:"agent_count" json:"agent_count"`
		Groups     []GroupBrief `bun:"-" json:"groups"`
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

	// Batch-load group memberships for all users
	if len(users) > 0 {
		userIDs := make([]int64, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
		}

		type memberRow struct {
			UserID    int64  `bun:"user_id"`
			GroupID   int64  `bun:"group_id"`
			GroupName string `bun:"group_name"`
			Role      string `bun:"role"`
		}
		var rows []memberRow
		_ = h.db.NewSelect().
			TableExpr("group_members AS gm").
			ColumnExpr("gm.user_id").
			ColumnExpr("gm.group_id").
			ColumnExpr("g.name AS group_name").
			ColumnExpr("gm.role").
			Join("JOIN groups AS g ON g.id = gm.group_id").
			Where("gm.user_id IN (?)", bun.In(userIDs)).
			Scan(ctx, &rows)

		groupMap := make(map[int64][]GroupBrief)
		for _, r := range rows {
			groupMap[r.UserID] = append(groupMap[r.UserID], GroupBrief{
				ID: r.GroupID, Name: r.GroupName, Role: r.Role,
			})
		}
		for i := range users {
			users[i].Groups = groupMap[users[i].ID]
			if users[i].Groups == nil {
				users[i].Groups = []GroupBrief{}
			}
		}
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
		Image         string `json:"image"`
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
			Image:         info.Image,
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

	// Delete the entire StatefulSet so it's recreated with latest settings
	// (resource limits, docker image, etc.) on next session creation
	if err := h.containerManager.DeleteStatefulSet(ctx, userIDStr, agentID); err != nil {
		log.Printf("Failed to delete StatefulSet for agent %d: %v", agentID, err)
		response.InternalError(c, "Failed to restart agent", err)
		return
	}

	log.Printf("Admin restarted agent %d (deleted StatefulSet) for user %d", agentID, userID)
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

func (h *Handler) UpdateAgentImage(c *gin.Context) {
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
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	userIDStr := fmt.Sprintf("%d", userID)

	// Verify agent exists
	var agent models.Agent
	err = h.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", agentID, userID).
		Scan(ctx)
	if err != nil {
		response.NotFound(c, "Agent not found")
		return
	}

	// Update StatefulSet image
	if err := h.containerManager.UpdateStatefulSetImage(ctx, userIDStr, agentID, req.Image); err != nil {
		log.Printf("Failed to update image for agent %d: %v", agentID, err)
		response.InternalError(c, "Failed to update agent image", err)
		return
	}

	// Mark active sessions as deleted (pod will restart)
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

	log.Printf("Admin updated agent %d image to %s for user %d", agentID, req.Image, userID)
	response.Success(c, "Agent image updated")
}

func (h *Handler) BatchUpdateImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()

	stsList, err := h.containerManager.ListStatefulSets(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list StatefulSets", err)
		return
	}

	type updateError struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	}

	var updated int
	var failed int
	var errors []updateError

	for i := range stsList.Items {
		sts := &stsList.Items[i]
		for j := range sts.Spec.Template.Spec.Containers {
			if sts.Spec.Template.Spec.Containers[j].Name == "claude-code" {
				sts.Spec.Template.Spec.Containers[j].Image = req.Image
				break
			}
		}
		_, err := h.containerManager.GetClientset().AppsV1().StatefulSets(sts.Namespace).Update(ctx, sts, metav1.UpdateOptions{})
		if err != nil {
			failed++
			errors = append(errors, updateError{Name: sts.Name, Error: err.Error()})
			log.Printf("Failed to update image for %s: %v", sts.Name, err)
		} else {
			updated++
			log.Printf("Updated image for %s to %s", sts.Name, req.Image)
		}
	}

	// Mark all active sessions as deleted
	_, _ = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	response.OK(c, gin.H{
		"total":   len(stsList.Items),
		"updated": updated,
		"failed":  failed,
		"errors":  errors,
	})
}

func (h *Handler) GetConversations(c *gin.Context) {
	ctx := context.Background()

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	type ConversationRow struct {
		models.ConversationHistory
		Username  string `bun:"username" json:"username"`
		AgentName string `bun:"agent_name" json:"agent_name"`
	}

	q := h.db.NewSelect().
		TableExpr("conversation_histories AS ch").
		ColumnExpr("ch.*").
		ColumnExpr("u.username AS username").
		ColumnExpr("a.name AS agent_name").
		Join("LEFT JOIN users AS u ON u.id = ch.user_id").
		Join("LEFT JOIN agents AS a ON a.id = ch.agent_id").
		OrderExpr("ch.timestamp DESC").
		Limit(limit)

	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("ch.user_id = ?", uid)
	}
	if aid := c.Query("agent_id"); aid != "" {
		q = q.Where("ch.agent_id = ?", aid)
	}
	if sid := c.Query("session_id"); sid != "" {
		q = q.Where("ch.session_id = ?", sid)
	}
	if before := c.Query("before"); before != "" {
		t, err := time.Parse(time.RFC3339Nano, before)
		if err == nil {
			q = q.Where("ch.timestamp < ?", t)
		}
	}
	if start := c.Query("start"); start != "" {
		if t, err := time.Parse(time.RFC3339Nano, start); err == nil {
			q = q.Where("ch.timestamp >= ?", t)
		}
	}
	if end := c.Query("end"); end != "" {
		if t, err := time.Parse(time.RFC3339Nano, end); err == nil {
			q = q.Where("ch.timestamp <= ?", t)
		}
	}

	var rows []ConversationRow
	err := q.Scan(ctx, &rows)
	if err != nil {
		response.InternalError(c, "Failed to fetch conversations", err)
		return
	}

	if rows == nil {
		rows = []ConversationRow{}
	}

	response.OK(c, gin.H{
		"conversations": rows,
		"count":         len(rows),
	})
}

func (h *Handler) ExportConversations(c *gin.Context) {
	ctx := context.Background()

	type ConversationRow struct {
		Timestamp time.Time `bun:"timestamp"`
		Username  string    `bun:"username"`
		AgentName string    `bun:"agent_name"`
		SessionID string    `bun:"session_id"`
		Role      string    `bun:"role"`
		Content   string    `bun:"content"`
	}

	q := h.db.NewSelect().
		TableExpr("conversation_histories AS ch").
		ColumnExpr("ch.timestamp").
		ColumnExpr("u.username").
		ColumnExpr("a.name AS agent_name").
		ColumnExpr("ch.session_id").
		ColumnExpr("ch.role").
		ColumnExpr("ch.content").
		Join("LEFT JOIN users AS u ON u.id = ch.user_id").
		Join("LEFT JOIN agents AS a ON a.id = ch.agent_id").
		OrderExpr("ch.timestamp DESC")

	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("ch.user_id = ?", uid)
	}
	if aid := c.Query("agent_id"); aid != "" {
		q = q.Where("ch.agent_id = ?", aid)
	}
	if sid := c.Query("session_id"); sid != "" {
		q = q.Where("ch.session_id = ?", sid)
	}
	if start := c.Query("start"); start != "" {
		if t, err := time.Parse(time.RFC3339Nano, start); err == nil {
			q = q.Where("ch.timestamp >= ?", t)
		}
	}
	if end := c.Query("end"); end != "" {
		if t, err := time.Parse(time.RFC3339Nano, end); err == nil {
			q = q.Where("ch.timestamp <= ?", t)
		}
	}

	var rows []ConversationRow
	err := q.Scan(ctx, &rows)
	if err != nil {
		response.InternalError(c, "Failed to export conversations", err)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=conversations_%s.csv", time.Now().Format("20060102_150405")))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"timestamp", "user", "agent", "session_id", "role", "content"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.Timestamp.Format(time.RFC3339),
			r.Username,
			r.AgentName,
			r.SessionID,
			r.Role,
			r.Content,
		})
	}
	w.Flush()
}

// RegisterRoutes registers admin routes on the given admin router group.
// The caller must provide a group that already has AdminMiddleware applied.
func (h *Handler) RegisterRoutes(adminGroup *gin.RouterGroup) {
	adminGroup.GET("/settings", h.GetSettings)
	adminGroup.PUT("/settings/:key", h.UpdateSetting)
	adminGroup.GET("/users", h.GetUsers)
	adminGroup.PUT("/users/:id/role", h.UpdateUserRole)
	adminGroup.GET("/users/:id/settings", h.GetUserSettings)
	adminGroup.PUT("/users/:id/settings/:key", h.SetUserSetting)
	adminGroup.DELETE("/users/:id/settings/:key", h.DeleteUserSetting)
	adminGroup.GET("/users/:id/agents", h.GetUserAgents)
	adminGroup.DELETE("/users/:id/agents/:agentId", h.DeleteUserAgent)
	adminGroup.POST("/users/:id/agents/:agentId/restart", h.RestartUserAgent)
	adminGroup.PUT("/users/:id/agents/:agentId/resources", h.UpdateAgentResources)
	adminGroup.PUT("/users/:id/agents/:agentId/image", h.UpdateAgentImage)
	adminGroup.POST("/agents/batch-update-image", h.BatchUpdateImage)
	adminGroup.GET("/conversations", h.GetConversations)
	adminGroup.GET("/conversations/export", h.ExportConversations)
}
