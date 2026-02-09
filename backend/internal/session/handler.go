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
}

func NewHandler(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService, settingsService *admin.SettingsService) *Handler {
	return &Handler{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
		settingsService:  settingsService,
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

// CreateSession creates a new session using a per-agent StatefulSet
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

	// Generate session ID
	sessionID := uuid.New().String()
	userIDInt := userID.(int64)
	userIDStr := fmt.Sprintf("%d", userIDInt)

	// Require agentID
	if req.AgentID <= 0 {
		response.BadRequest(c, "agent_id is required")
		return
	}

	log.Printf("Creating session: userID=%s, sessionID=%s, agentID=%d", userIDStr, sessionID, req.AgentID)

	// Auto-close any existing running sessions for this user
	_, err := h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ?", userIDInt).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusCreating,
			models.SessionStatusIdle,
		})).
		Exec(ctx)
	if err != nil {
		log.Printf("Warning: failed to auto-close old sessions: %v", err)
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

		// Get resource limits from settings
		limits := h.settingsService.GetResourceLimits(ctx, userIDInt)
		rc := &container.ResourceConfig{
			CPURequest:    limits.CPURequest,
			CPULimit:      limits.CPULimit,
			MemoryRequest: limits.MemoryRequest,
			MemoryLimit:   limits.MemoryLimit,
		}

		// Create StatefulSet with headless service
		if err := h.containerManager.CreateStatefulSet(ctx, userIDStr, req.AgentID, agent.Config, rc); err != nil {
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
