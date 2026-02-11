package session

import (
	"context"
	"fmt"
	"log"
	"time"

	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/workspace"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Handler struct {
	db               *bun.DB
	containerManager *container.Manager
	syncService      *skill.SyncService
	settingsService  *admin.SettingsService
	workspaceSyncSvc *workspace.SyncService
}

func NewHandler(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService, settingsService *admin.SettingsService, workspaceSyncSvc *workspace.SyncService) *Handler {
	return &Handler{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
		settingsService:  settingsService,
		workspaceSyncSvc: workspaceSyncSvc,
	}
}

type CreateSessionRequest struct {
	AgentID int64 `json:"agent_id"` // Optional: which agent to use
}

type CreateSessionResponse struct {
	SessionID string               `json:"session_id"`
	Status    models.SessionStatus `json:"status"`
	PodName   string               `json:"pod_name,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
}

// CreateSession creates or reuses a session using a per-agent StatefulSet.
// If an active session already exists for the same user+agent pair with a
// healthy pod, it is returned directly (GetOrCreate semantics). This allows
// the frontend to reconnect to the same terminal after a WS disconnect.
func (h *Handler) CreateSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Agent ID is optional, so ignore bind errors
	}

	ctx := context.Background()

	userIDInt := userID.(int64)
	userIDStr := fmt.Sprintf("%d", userIDInt)

	// Require agentID
	if req.AgentID <= 0 {
		response.BadRequest(c, "agent_id is required")
		return
	}

	// --- GetOrCreate: try to reuse an existing session for this user+agent ---
	var existing models.Session
	err := h.db.NewSelect().
		Model(&existing).
		Where("user_id = ?", userIDInt).
		Where("agent_id = ?", req.AgentID).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusIdle,
		})).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)

	if err == nil {
		// Found an existing session — verify the pod is still healthy
		podIP, podErr := h.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentID)
		if podErr == nil && podIP != "" {
			// Pod is healthy — reuse session, refresh pod_ip and last_active
			now := time.Now()
			_, _ = h.db.NewUpdate().
				Model((*models.Session)(nil)).
				Set("pod_ip = ?", podIP).
				Set("last_active = ?", now).
				Set("status = ?", models.SessionStatusRunning).
				Set("updated_at = ?", now).
				Where("id = ?", existing.ID).
				Exec(ctx)

			log.Printf("Reusing existing session: sessionID=%s, agentID=%d, podIP=%s", existing.SessionID, req.AgentID, podIP)

			response.Created(c, CreateSessionResponse{
				SessionID: existing.SessionID,
				Status:    models.SessionStatusRunning,
				PodName:   existing.PodName,
				CreatedAt: existing.CreatedAt,
			})
			return
		}

		// Pod is unhealthy — mark old session as deleted, fall through to create new
		log.Printf("Existing session %s has unhealthy pod, marking as deleted", existing.SessionID)
		_, _ = h.db.NewUpdate().
			Model((*models.Session)(nil)).
			Set("status = ?", models.SessionStatusDeleted).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", existing.ID).
			Exec(ctx)
	}

	// --- Create new session ---
	sessionID := uuid.New().String()
	log.Printf("Creating session: userID=%s, sessionID=%s, agentID=%d", userIDStr, sessionID, req.AgentID)

	// Close sessions for OTHER agents (user can only have one active agent at a time)
	_, err = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ?", userIDInt).
		Where("agent_id != ?", req.AgentID).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusCreating,
			models.SessionStatusIdle,
		})).
		Exec(ctx)
	if err != nil {
		log.Printf("Warning: failed to auto-close other agent sessions: %v", err)
		// Not fatal, continue
	}

	// Load agent configuration
	var agent models.Agent
	err = h.db.NewSelect().
		Model(&agent).
		Where("id = ?", req.AgentID).
		Where("created_by = ?", userIDInt).
		Scan(ctx)

	if err != nil {
		log.Printf("Failed to load agent %d: %v", req.AgentID, err)
		response.NotFound(c, "Agent not found", err)
		return
	}

	log.Printf("Using agent: %s (ID: %d)", agent.Name, agent.ID)

	// Check if StatefulSet exists for this user-agent combination
	sts, err := h.containerManager.GetStatefulSet(ctx, userIDStr, req.AgentID)
	if err != nil {
		log.Printf("StatefulSet not found, creating it...")

		// Get resource limits from settings (user/system defaults)
		limits := h.settingsService.GetResourceLimits(ctx, userIDInt)
		rc := &container.ResourceConfig{
			CPURequest:    limits.CPURequest,
			CPULimit:      limits.CPULimit,
			MemoryRequest: limits.MemoryRequest,
			MemoryLimit:   limits.MemoryLimit,
		}
		// Agent-level overrides take priority
		if agent.CPURequest != nil {
			rc.CPURequest = *agent.CPURequest
		}
		if agent.CPULimit != nil {
			rc.CPULimit = *agent.CPULimit
		}
		if agent.MemoryRequest != nil {
			rc.MemoryRequest = *agent.MemoryRequest
		}
		if agent.MemoryLimit != nil {
			rc.MemoryLimit = *agent.MemoryLimit
		}

		// Get docker image from settings (empty string falls back to env default)
		dockerImage := h.settingsService.GetDockerImage(ctx)

		// Create StatefulSet with headless service
		if err := h.containerManager.CreateStatefulSet(ctx, userIDStr, req.AgentID, agent.Config, rc, dockerImage); err != nil {
			log.Printf("Failed to create StatefulSet: %v", err)
			response.InternalError(c, "Failed to create StatefulSet", err)
			return
		}

		log.Printf("StatefulSet and headless service created, waiting for pod to be ready...")

		// Wait for pod to be ready before returning
		if err := h.containerManager.WaitForStatefulSetReady(ctx, userIDStr, req.AgentID, 60, 5*time.Second); err != nil {
			log.Printf("Warning: %v", err)
			// Don't fail — pod may still be starting, session can be used once ready
		}
	} else {
		log.Printf("Using existing StatefulSet: %s", sts.Name)
	}

	// Get the actual Pod IP
	podIP, err := h.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentID)
	if err != nil {
		log.Printf("Failed to get Pod IP: %v", err)
		response.InternalError(c, "Failed to get Pod IP, pod may not be ready", err)
		return
	}
	log.Printf("Pod IP: %s", podIP)

	// Sync workspace files from OSS to the pod (private + public + claude-commands)
	if h.workspaceSyncSvc != nil {
		if err := h.workspaceSyncSvc.SyncWorkspaceToPod(ctx, userIDStr, req.AgentID); err != nil {
			log.Printf("Warning: failed to sync workspace: %v", err)
			// Not fatal — session can still work without workspace files
		}
	}

	// Sync all installed skills to the pod (ensures files exist after pod restart)
	if err := h.syncService.SyncAllSkillsToAgent(ctx, userIDStr, req.AgentID); err != nil {
		log.Printf("Warning: failed to sync skills to agent %d: %v", req.AgentID, err)
		// Not fatal — session can still work, just without slash commands
	}

	// Save session to database
	stsName := fmt.Sprintf("claude-code-%s-%d", userIDStr, req.AgentID)
	session := &models.Session{
		UserID:     userIDInt,
		AgentID:    req.AgentID,
		SessionID:  sessionID,
		PodName:    stsName, // StatefulSet name
		PodIP:      podIP,   // Actual Pod IP
		Status:     models.SessionStatusRunning,
		LastActive: time.Now(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = h.db.NewInsert().Model(session).Exec(ctx)
	if err != nil {
		log.Printf("Failed to save session to database: %v", err)
		response.InternalError(c, "Failed to save session", err)
		return
	}

	response.Created(c, CreateSessionResponse{
		SessionID: sessionID,
		Status:    models.SessionStatusRunning,
		PodName:   stsName,
		CreatedAt: session.CreatedAt,
	})
}

// GetSession retrieves a session by ID
func (h *Handler) GetSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	ctx := context.Background()
	var session models.Session
	err := h.db.NewSelect().
		Model(&session).
		Where("session_id = ?", sessionID).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		log.Printf("Session not found: %v", err)
		response.NotFound(c, "Session not found", err)
		return
	}

	response.OK(c, session)
}

// ListSessions lists all sessions for the current user
func (h *Handler) ListSessions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	ctx := context.Background()
	var sessions []models.Session
	err := h.db.NewSelect().
		Model(&sessions).
		Where("user_id = ?", userID).
		Where("status != ?", models.SessionStatusDeleted).
		Order("created_at DESC").
		Scan(ctx)

	if err != nil {
		log.Printf("Failed to list sessions: %v", err)
		response.InternalError(c, "Failed to list sessions", err)
		return
	}

	response.OK(c, sessions)
}

// DeleteSession soft-deletes a session (marks as deleted).
// The StatefulSet is NOT deleted because other sessions may share it.
func (h *Handler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	ctx := context.Background()

	// Get session
	var session models.Session
	err := h.db.NewSelect().
		Model(&session).
		Where("session_id = ?", sessionID).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		response.NotFound(c, "Session not found", err)
		return
	}

	// Soft-delete: mark session as deleted
	_, err = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update session status: %v", err)
		response.InternalError(c, "Failed to delete session", err)
		return
	}

	response.Success(c, "Session deleted successfully")
}

// RegisterRoutes registers session routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	sessions := rg.Group("/sessions")
	{
		sessions.POST("", h.CreateSession)
		sessions.GET("", h.ListSessions)
		sessions.GET("/:sessionId", h.GetSession)
		sessions.DELETE("/:sessionId", h.DeleteSession)
	}
}
