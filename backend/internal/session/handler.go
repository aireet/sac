package session

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/echotech/sac/internal/container"
	"github.com/echotech/sac/internal/models"
	"github.com/echotech/sac/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Handler struct {
	db               *bun.DB
	containerManager *container.Manager
}

func NewHandler(db *bun.DB, cfg *config.Config) (*Handler, error) {
	// Initialize container manager
	mgr, err := container.NewManager(
		cfg.KubeconfigPath,
		cfg.Namespace, // use namespace from config
		cfg.DockerRegistry,
		cfg.DockerImage,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container manager: %w", err)
	}

	return &Handler{
		db:               db,
		containerManager: mgr,
	}, nil
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

// CreateSession creates a new session using the shared Claude Code deployment
func (h *Handler) CreateSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id is required"})
		return
	}

	log.Printf("Creating session: userID=%s, sessionID=%s, agentID=%d", userIDStr, sessionID, req.AgentID)

	// Load agent configuration
	var agent models.Agent
	err := h.db.NewSelect().
		Model(&agent).
		Where("id = ?", req.AgentID).
		Where("created_by = ?", userIDInt).
		Scan(ctx)

	if err != nil {
		log.Printf("Failed to load agent %d: %v", req.AgentID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	log.Printf("Using agent: %s (ID: %d)", agent.Name, agent.ID)

	// Check if deployment exists for this user-agent combination
	deployment, err := h.containerManager.GetDeployment(ctx, userIDStr, req.AgentID)
	if err != nil {
		log.Printf("Deployment not found, creating it...")

		// Create deployment with agent config
		if err := h.containerManager.CreateDeployment(ctx, userIDStr, req.AgentID, agent.Config); err != nil {
			log.Printf("Failed to create deployment: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create deployment"})
			return
		}

		// Create service
		if err := h.containerManager.CreateService(ctx, userIDStr, req.AgentID); err != nil {
			log.Printf("Failed to create service: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create service"})
			return
		}

		log.Printf("Deployment and Service created successfully")
	} else {
		log.Printf("Using existing deployment: %s", deployment.Name)
	}

	// Get service ClusterIP
	serviceIP, err := h.containerManager.GetServiceClusterIP(ctx, userIDStr, req.AgentID)
	if err != nil {
		log.Printf("Failed to get service IP: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get service IP"})
		return
	}

	log.Printf("Service ClusterIP: %s", serviceIP)

	// Save session to database
	deploymentName := fmt.Sprintf("claude-code-%s-%d", userIDStr, req.AgentID)
	session := &models.Session{
		UserID:     userIDInt,
		SessionID:  sessionID,
		PodName:    deploymentName, // Deployment name
		PodIP:      serviceIP,      // Use service ClusterIP
		Status:     models.SessionStatusRunning,
		LastActive: time.Now(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = h.db.NewInsert().Model(session).Exec(ctx)
	if err != nil {
		log.Printf("Failed to save session to database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
		return
	}

	c.JSON(http.StatusCreated, CreateSessionResponse{
		SessionID: sessionID,
		Status:    models.SessionStatusRunning,
		PodName:   deploymentName,
		CreatedAt: session.CreatedAt,
	})
}

// waitForPodReady waits for the pod to be ready and updates the session
func (h *Handler) waitForPodReady(ctx context.Context, userID, sessionID string) {
	maxRetries := 60 // Wait up to 5 minutes
	retryInterval := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryInterval)

		// Get pod IP
		podIP, err := h.containerManager.GetPodIP(ctx, userID, sessionID)
		if err != nil {
			log.Printf("Waiting for pod IP (attempt %d/%d): %v", i+1, maxRetries, err)
			continue
		}

		// Update session with pod IP
		_, err = h.db.NewUpdate().
			Model(&models.Session{}).
			Set("pod_ip = ?", podIP).
			Set("status = ?", models.SessionStatusRunning).
			Set("updated_at = ?", time.Now()).
			Where("session_id = ?", sessionID).
			Exec(ctx)

		if err != nil {
			log.Printf("Failed to update session with pod IP: %v", err)
			return
		}

		log.Printf("Session %s is ready with pod IP: %s", sessionID, podIP)
		return
	}

	// Timeout
	log.Printf("Timeout waiting for pod to be ready: %s", sessionID)
	h.db.NewUpdate().
		Model(&models.Session{}).
		Set("status = ?", models.SessionStatusStopped).
		Where("session_id = ?", sessionID).
		Exec(ctx)
}

// GetSession retrieves a session by ID
func (h *Handler) GetSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// ListSessions lists all sessions for the current user
func (h *Handler) ListSessions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list sessions"})
		return
	}

	c.JSON(http.StatusOK, sessions)
}

// DeleteSession deletes a session and its pod
func (h *Handler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	userIDStr := fmt.Sprintf("%d", userID.(int64))

	// Delete pod
	if err := h.containerManager.DeletePod(ctx, userIDStr, sessionID); err != nil {
		log.Printf("Failed to delete pod: %v", err)
		// Continue anyway
	}

	// Update session status
	_, err = h.db.NewUpdate().
		Model(&models.Session{}).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update session status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Session deleted successfully"})
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
