package session

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/workspace"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
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

	req := &sacv1.CreateSessionRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	ctx := context.Background()

	userIDInt := userID.(int64)
	userIDStr := fmt.Sprintf("%d", userIDInt)

	// Require agentID
	if req.AgentId <= 0 {
		response.BadRequest(c, "agent_id is required")
		return
	}

	// --- GetOrCreate: try to reuse an existing session for this user+agent ---
	var existing models.Session
	err := h.db.NewSelect().
		Model(&existing).
		Where("user_id = ?", userIDInt).
		Where("agent_id = ?", req.AgentId).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusIdle,
		})).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)

	if err == nil {
		// Found an existing session — verify the pod is still healthy
		podIP, podErr := h.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentId)
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

			log.Printf("Reusing existing session: sessionID=%s, agentID=%d, podIP=%s", existing.SessionID, req.AgentId, podIP)

			protobind.Created(c, &sacv1.CreateSessionResponse{
				SessionId: existing.SessionID,
				Status:    string(models.SessionStatusRunning),
				PodName:   existing.PodName,
				CreatedAt: timestamppb.New(existing.CreatedAt),
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
	log.Printf("Creating session: userID=%s, sessionID=%s, agentID=%d", userIDStr, sessionID, req.AgentId)

	// Close sessions for OTHER agents (user can only have one active agent at a time)
	_, err = h.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ?", userIDInt).
		Where("agent_id != ?", req.AgentId).
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
		Where("id = ?", req.AgentId).
		Where("created_by = ?", userIDInt).
		Scan(ctx)

	if err != nil {
		log.Printf("Failed to load agent %d: %v", req.AgentId, err)
		response.NotFound(c, "Agent not found", err)
		return
	}

	log.Printf("Using agent: %s (ID: %d)", agent.Name, agent.ID)

	// Check if StatefulSet exists for this user-agent combination
	isNewStatefulSet := false
	sts, err := h.containerManager.GetStatefulSet(ctx, userIDStr, req.AgentId)
	if err != nil {
		isNewStatefulSet = true
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
		if err := h.containerManager.CreateStatefulSet(ctx, userIDStr, req.AgentId, agent.Config, rc, dockerImage); err != nil {
			log.Printf("Failed to create StatefulSet: %v", err)
			response.InternalError(c, "Failed to create StatefulSet", err)
			return
		}

		log.Printf("StatefulSet and headless service created, waiting for pod to be ready...")

		// Wait for pod to be ready before returning
		if err := h.containerManager.WaitForStatefulSetReady(ctx, userIDStr, req.AgentId, 60, 5*time.Second); err != nil {
			log.Printf("Warning: %v", err)
			// Don't fail — pod may still be starting, session can be used once ready
		}
	} else {
		log.Printf("Using existing StatefulSet: %s", sts.Name)
	}

	// Get the actual Pod IP
	podIP, err := h.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentId)
	if err != nil {
		log.Printf("Failed to get Pod IP: %v", err)
		response.InternalError(c, "Failed to get Pod IP, pod may not be ready", err)
		return
	}
	log.Printf("Pod IP: %s", podIP)

	if isNewStatefulSet {
		// New pod — only sync skills (fast). Workspace sync is deferred to
		// the frontend which calls the SSE /workspace/sync-stream endpoint
		// with a progress bar so the user sees what's happening.
		if err := h.syncService.SyncAllSkillsToAgent(ctx, userIDStr, req.AgentId); err != nil {
			log.Printf("Warning: failed to sync skills to agent %d: %v", req.AgentId, err)
		}
		// Write merged CLAUDE.md (system instructions + agent instructions) to pod
		h.writeClaudeMD(ctx, userIDStr, req.AgentId, agent.Instructions)
	} else {
		// Existing pod — files already on disk from last sync. Run background
		// sync to pick up any changes without blocking session creation.
		go func() {
			bgCtx := context.Background()
			if h.workspaceSyncSvc != nil {
				if err := h.workspaceSyncSvc.SyncWorkspaceToPod(bgCtx, userIDStr, req.AgentId); err != nil {
					log.Printf("Warning: background workspace sync failed: %v", err)
				}
			}
			if err := h.syncService.SyncAllSkillsToAgent(bgCtx, userIDStr, req.AgentId); err != nil {
				log.Printf("Warning: background skill sync failed: %v", err)
			}
			// Write merged CLAUDE.md (system instructions + agent instructions) to pod
			h.writeClaudeMD(bgCtx, userIDStr, req.AgentId, agent.Instructions)
			log.Printf("Background sync completed for user %s agent %d", userIDStr, req.AgentId)
		}()
	}

	// Save session to database
	stsName := fmt.Sprintf("claude-code-%s-%d", userIDStr, req.AgentId)
	session := &models.Session{
		UserID:     userIDInt,
		AgentID:    req.AgentId,
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

	protobind.Created(c, &sacv1.CreateSessionResponse{
		SessionId: sessionID,
		Status:    string(models.SessionStatusRunning),
		PodName:   stsName,
		CreatedAt: timestamppb.New(session.CreatedAt),
		IsNew:     isNewStatefulSet,
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

	protobind.OK(c, convert.SessionToProto(&session))
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

	protobind.OK(c, &sacv1.UserSessionListResponse{Sessions: convert.SessionsToProto(sessions)})
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

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Session deleted successfully"})
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

// getGroupTemplates returns non-empty CLAUDE.md templates from all groups the user belongs to, sorted by group name.
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

// writeClaudeMD writes the merged CLAUDE.md (system + group templates + agent instructions) to the pod.
func (h *Handler) writeClaudeMD(ctx context.Context, userIDStr string, agentID int64, agentInstructions string) {
	sysInstructions := h.settingsService.GetAgentSystemInstructions(ctx)
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	groupTemplates := h.getGroupTemplates(ctx, userID)

	if sysInstructions == "" && len(groupTemplates) == 0 && agentInstructions == "" {
		return
	}

	var parts []string
	if sysInstructions != "" {
		parts = append(parts, sysInstructions)
	}
	parts = append(parts, groupTemplates...)
	if agentInstructions != "" {
		parts = append(parts, agentInstructions)
	}

	content := strings.Join(parts, "\n\n---\n\n")
	podName := fmt.Sprintf("claude-code-%s-%d-0", userIDStr, agentID)
	if err := h.containerManager.WriteFileInPod(ctx, podName, "/workspace/CLAUDE.md", content); err != nil {
		log.Printf("Warning: failed to write CLAUDE.md to pod %s: %v", podName, err)
	}
}
