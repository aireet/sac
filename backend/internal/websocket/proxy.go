package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/models"
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
	db         *bun.DB
	jwtService *auth.JWTService
}

func NewProxyHandler(db *bun.DB, jwtService *auth.JWTService) *ProxyHandler {
	return &ProxyHandler{
		db:         db,
		jwtService: jwtService,
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
	sessionID := c.Param("sessionId")

	// Authenticate via JWT token in query param
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token query parameter required"})
		return
	}

	claims, err := h.jwtService.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	userID := fmt.Sprintf("%d", claims.UserID)
	agentIDStr := c.Query("agent_id")

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
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
		Where("status != ?", "deleted").
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

	// Step 1: Wait for client's first message to get actual terminal dimensions.
	// The frontend sends a JSON resize message immediately on connection:
	//   {"type":"resize","columns":N,"rows":N}
	// We use these dimensions in the ttyd auth handshake so the PTY starts
	// at the correct size, preventing output misalignment.
	columns, rows := 100, 30 // sensible defaults
	_, firstMsg, err := clientConn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read initial message from client: %v", err)
		return
	}
	if len(firstMsg) > 0 && firstMsg[0] == '{' {
		var msg struct {
			Type    string `json:"type"`
			Columns int    `json:"columns"`
			Rows    int    `json:"rows"`
		}
		if json.Unmarshal(firstMsg, &msg) == nil && msg.Columns > 0 && msg.Rows > 0 {
			columns = msg.Columns
			rows = msg.Rows
			log.Printf("Client reported terminal size: %dx%d", columns, rows)
		}
	}

	// Connect to ttyd in the pod
	ttydURL := fmt.Sprintf("ws://%s:7681/ws", session.PodIP)
	log.Printf("Connecting to ttyd at: %s", ttydURL)

	// ttyd requires the "tty" WebSocket subprotocol
	ttydHeaders := http.Header{}
	ttydHeaders.Set("Sec-WebSocket-Protocol", "tty")
	ttydConn, _, err := websocket.DefaultDialer.Dial(ttydURL, ttydHeaders)
	if err != nil {
		log.Printf("Failed to connect to ttyd: %v", err)
		clientConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: Failed to connect to container: %v", err)))
		return
	}
	defer ttydConn.Close()

	log.Printf("Connected to ttyd successfully")

	// Send authentication handshake with the client's actual terminal dimensions
	authMsg := fmt.Sprintf(`{"AuthToken":"","columns":%d,"rows":%d}`, columns, rows)
	if err := ttydConn.WriteMessage(websocket.BinaryMessage, []byte(authMsg)); err != nil {
		log.Printf("Failed to send auth handshake: %v", err)
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to authenticate with container"))
		return
	}
	log.Printf("Sent ttyd auth handshake with dimensions: %dx%d", columns, rows)

	// Update last active time
	_, err = h.db.NewUpdate().
		Model(&session).
		Set("last_active = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)
	if err != nil {
		log.Printf("Failed to update last_active: %v", err)
	}

	// Start bidirectional forwarding with ttyd protocol translation
	var wg sync.WaitGroup
	wg.Add(2)

	// Forward messages from client to ttyd (wrap as ttyd INPUT messages)
	go func() {
		defer wg.Done()
		defer ttydConn.Close()
		h.forwardClientToTtyd(clientConn, ttydConn)
	}()

	// Forward messages from ttyd to client (extract terminal output)
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		h.forwardTtydToClient(ttydConn, clientConn)
	}()

	// Wait for both directions to complete
	wg.Wait()
	log.Printf("WebSocket proxy closed for session: %s", sessionID)
}

// ttyd WebSocket protocol uses ASCII character bytes as message type prefixes.
// Client -> Server:
//
//	'0' (0x30) = INPUT (terminal input data)
//	'1' (0x31) = RESIZE_TERMINAL (JSON: {"columns":N,"rows":N})
//	'{' (0x7B) = JSON_DATA (initial auth handshake)
//
// Server -> Client:
//
//	'0' (0x30) = OUTPUT (terminal output data)
//	'1' (0x31) = SET_WINDOW_TITLE
//	'2' (0x32) = SET_PREFERENCES
const (
	ttydInput          byte = '0' // Client -> Server: terminal input
	ttydResizeTerminal byte = '1' // Client -> Server: resize terminal
	ttydOutput         byte = '0' // Server -> Client: terminal output
	ttydSetWindowTitle byte = '1' // Server -> Client: set window title
	ttydSetPreferences byte = '2' // Server -> Client: set preferences
)

// forwardClientToTtyd wraps client messages as ttyd binary messages.
// Supports two message types from the frontend:
//   - JSON with "type":"resize" → ttyd RESIZE_TERMINAL message
//   - Everything else → ttyd INPUT message
func (h *ProxyHandler) forwardClientToTtyd(src, dst *websocket.Conn) {
	for {
		_, message, err := src.ReadMessage()
		if err != nil {
			if err != io.EOF && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Error reading message (client->ttyd): %v", err)
			}
			return
		}

		// Check if this is a resize message from the frontend
		if len(message) > 0 && message[0] == '{' {
			var msg struct {
				Type    string `json:"type"`
				Columns int    `json:"columns"`
				Rows    int    `json:"rows"`
			}
			if json.Unmarshal(message, &msg) == nil && msg.Type == "resize" {
				resizeJSON := fmt.Sprintf(`{"columns":%d,"rows":%d}`, msg.Columns, msg.Rows)
				wrapped := append([]byte{ttydResizeTerminal}, []byte(resizeJSON)...)
				if err := dst.WriteMessage(websocket.BinaryMessage, wrapped); err != nil {
					log.Printf("Error writing resize (client->ttyd): %v", err)
					return
				}
				continue
			}
		}

		// Wrap as ttyd INPUT message: ASCII '0' + data
		wrapped := make([]byte, len(message)+1)
		wrapped[0] = ttydInput
		copy(wrapped[1:], message)

		if err := dst.WriteMessage(websocket.BinaryMessage, wrapped); err != nil {
			log.Printf("Error writing message (client->ttyd): %v", err)
			return
		}
	}
}

// forwardTtydToClient extracts terminal output from ttyd binary messages and forwards as text
func (h *ProxyHandler) forwardTtydToClient(src, dst *websocket.Conn) {
	for {
		_, message, err := src.ReadMessage()
		if err != nil {
			if err != io.EOF && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Error reading message (ttyd->client): %v", err)
			}
			return
		}

		if len(message) < 1 {
			continue
		}

		msgType := message[0]
		payload := message[1:]

		switch msgType {
		case ttydOutput: // Terminal output - forward as binary to preserve raw PTY bytes
			if err := dst.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				log.Printf("Error writing message (ttyd->client): %v", err)
				return
			}
		case ttydSetWindowTitle, ttydSetPreferences:
			// Ignore window title and preferences messages
		default:
			log.Printf("Skipping unknown ttyd message type: %d", msgType)
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
