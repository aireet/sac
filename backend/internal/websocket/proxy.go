package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/rs/zerolog/log"
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
		log.Warn().Err(err).Msg("failed to upgrade WebSocket connection")
		return
	}
	defer clientConn.Close()

	log.Info().Str("user_id", userID).Str("session_id", sessionID).Str("agent_id", agentIDStr).Msg("client connected")

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
			log.Debug().Str("agent", agent.Name).Msg("loaded agent")
		} else {
			log.Warn().Err(err).Str("agent_id", agentIDStr).Msg("failed to load agent")
		}
	}
	var session models.Session
	err = h.db.NewSelect().
		Model(&session).
		Where("session_id = ?", sessionID).
		Where("status != ?", "deleted").
		Scan(ctx)

	if err != nil {
		log.Warn().Err(err).Msg("failed to find session")
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Session not found"))
		return
	}

	if session.PodIP == "" {
		log.Warn().Str("session_id", sessionID).Msg("pod IP not available")
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
		log.Warn().Err(err).Msg("failed to read initial message from client")
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
			log.Debug().Int("columns", columns).Int("rows", rows).Msg("client reported terminal size")
		}
	}

	// Connect to ttyd in the pod
	ttydURL := fmt.Sprintf("ws://%s:7681/ws", session.PodIP)
	log.Debug().Str("url", ttydURL).Msg("connecting to ttyd")

	// ttyd requires the "tty" WebSocket subprotocol
	ttydHeaders := http.Header{}
	ttydHeaders.Set("Sec-WebSocket-Protocol", "tty")
	ttydConn, _, err := websocket.DefaultDialer.Dial(ttydURL, ttydHeaders)
	if err != nil {
		log.Warn().Err(err).Msg("failed to connect to ttyd")
		clientConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: Failed to connect to container: %v", err)))
		return
	}
	defer ttydConn.Close()

	log.Debug().Msg("connected to ttyd")

	// Send authentication handshake with the client's actual terminal dimensions
	authMsg := fmt.Sprintf(`{"AuthToken":"","columns":%d,"rows":%d}`, columns, rows)
	if err := ttydConn.WriteMessage(websocket.BinaryMessage, []byte(authMsg)); err != nil {
		log.Warn().Err(err).Msg("failed to send auth handshake")
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to authenticate with container"))
		return
	}
	log.Debug().Int("columns", columns).Int("rows", rows).Msg("sent ttyd auth handshake")

	// Update last active time
	_, err = h.db.NewUpdate().
		Model(&session).
		Set("last_active = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to update last_active")
	}

	// Enable ping/pong heartbeat to prevent idle disconnections (Envoy/NAT timeout).
	// PongHandler refreshes the read deadline on each pong response so the
	// connection stays alive as long as the peer is responsive.
	const heartbeatInterval = 30 * time.Second
	const pongTimeout = 60 * time.Second

	clientConn.SetReadDeadline(time.Now().Add(pongTimeout))
	clientConn.SetPongHandler(func(string) error {
		clientConn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	ttydConn.SetReadDeadline(time.Now().Add(pongTimeout))
	ttydConn.SetPongHandler(func(string) error {
		ttydConn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	// Start heartbeat goroutines for both directions
	go h.StartHeartbeat(clientConn, heartbeatInterval)
	go h.StartHeartbeat(ttydConn, heartbeatInterval)

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
	log.Info().Str("session_id", sessionID).Msg("WebSocket proxy closed")
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
				log.Debug().Err(err).Msg("error reading message (client->ttyd)")
			}
			return
		}
		// Refresh read deadline on successful read (data = activity)
		src.SetReadDeadline(time.Now().Add(60 * time.Second))

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
					log.Debug().Err(err).Msg("error writing resize (client->ttyd)")
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
			log.Debug().Err(err).Msg("error writing message (client->ttyd)")
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
				log.Debug().Err(err).Msg("error reading message (ttyd->client)")
			}
			return
		}
		// Refresh read deadline on successful read (data = activity)
		src.SetReadDeadline(time.Now().Add(60 * time.Second))

		if len(message) < 1 {
			continue
		}

		msgType := message[0]
		payload := message[1:]

		switch msgType {
		case ttydOutput: // Terminal output - forward as binary to preserve raw PTY bytes
			if err := dst.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				log.Debug().Err(err).Msg("error writing message (ttyd->client)")
				return
			}
		case ttydSetWindowTitle, ttydSetPreferences:
			// Ignore window title and preferences messages
		default:
			log.Debug().Uint8("type", msgType).Msg("skipping unknown ttyd message type")
		}
	}
}

// StartHeartbeat starts a heartbeat goroutine to keep connection alive
func (h *ProxyHandler) StartHeartbeat(conn *websocket.Conn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
			log.Debug().Err(err).Msg("failed to send ping")
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
