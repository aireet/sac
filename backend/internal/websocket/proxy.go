package websocket

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/echotech/sac/internal/database"
	"github.com/echotech/sac/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/uptrace/bun"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

type ProxyHandler struct {
	db *bun.DB
}

func NewProxyHandler(db *bun.DB) *ProxyHandler {
	return &ProxyHandler{
		db: db,
	}
}

// getConfigKeys returns the keys of the agent config map
func getConfigKeys(config map[string]interface{}) []string {
	if config == nil {
		return []string{}
	}
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	return keys
}

// HandleWebSocket handles WebSocket proxy connections
func (h *ProxyHandler) HandleWebSocket(c *gin.Context) {
	userID := c.Param("userId")
	sessionID := c.Param("sessionId")
	agentIDStr := c.Query("agent_id")

	if userID == "" || sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId and sessionId are required"})
		return
	}

	// Upgrade connection to WebSocket
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("Client connected: userID=%s, sessionID=%s, agentID=%s", userID, sessionID, agentIDStr)

	// Get Pod IP from database
	ctx := context.Background()

	// Load Agent configuration if provided
	var agent *models.Agent
	if agentIDStr != "" {
		var a models.Agent
		err := h.db.NewSelect().
			Model(&a).
			Where("id = ?", agentIDStr).
			Scan(ctx)
		if err == nil {
			agent = &a
			log.Printf("Loaded agent: %s (config keys: %v)", agent.Name, getConfigKeys(agent.Config))
		} else {
			log.Printf("Warning: Failed to load agent %s: %v", agentIDStr, err)
		}
	}
	var session models.Session
	err = h.db.NewSelect().
		Model(&session).
		Where("session_id = ?", sessionID).
		Scan(ctx)

	if err != nil {
		log.Printf("Failed to find session: %v", err)
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Session not found"))
		return
	}

	if session.PodIP == "" {
		log.Printf("Pod IP not available for session: %s", sessionID)
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Pod is not ready yet"))
		return
	}

	// Connect to ttyd in the pod
	ttydURL := fmt.Sprintf("ws://%s:7681/ws", session.PodIP)
	log.Printf("Connecting to ttyd at: %s", ttydURL)

	ttydConn, _, err := websocket.DefaultDialer.Dial(ttydURL, nil)
	if err != nil {
		log.Printf("Failed to connect to ttyd: %v", err)
		clientConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: Failed to connect to container: %v", err)))
		return
	}
	defer ttydConn.Close()

	log.Printf("Connected to ttyd successfully")

	// Update last active time
	_, err = h.db.NewUpdate().
		Model(&session).
		Set("last_active = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)
	if err != nil {
		log.Printf("Failed to update last_active: %v", err)
	}

	// Start bidirectional forwarding
	var wg sync.WaitGroup
	wg.Add(2)

	// Forward messages from client to ttyd
	go func() {
		defer wg.Done()
		defer ttydConn.Close()
		h.forwardMessages(clientConn, ttydConn, "client->ttyd")
	}()

	// Forward messages from ttyd to client
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		h.forwardMessages(ttydConn, clientConn, "ttyd->client")
	}()

	// Wait for both directions to complete
	wg.Wait()
	log.Printf("WebSocket proxy closed for session: %s", sessionID)
}

// forwardMessages forwards messages from source to destination
func (h *ProxyHandler) forwardMessages(src, dst *websocket.Conn, direction string) {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if err != io.EOF && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Error reading message (%s): %v", direction, err)
			}
			return
		}

		err = dst.WriteMessage(messageType, message)
		if err != nil {
			log.Printf("Error writing message (%s): %v", direction, err)
			return
		}
	}
}

// StartHeartbeat starts a heartbeat goroutine to keep connection alive
func (h *ProxyHandler) StartHeartbeat(conn *websocket.Conn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
			log.Printf("Failed to send ping: %v", err)
			return
		}
	}
}

// HealthCheck checks if the proxy service is healthy
func (h *ProxyHandler) HealthCheck(c *gin.Context) {
	ctx := context.Background()
	if err := database.HealthCheck(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "unhealthy",
			"database": "disconnected",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}
