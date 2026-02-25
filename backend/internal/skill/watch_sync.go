package skill

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"g.echo.tech/dev/sac/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var syncWsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WatchSync is a WebSocket endpoint that pushes skill sync progress events to the client.
// JWT is read from the "token" query parameter.
func WatchSync(hub *SyncHub, jwtService *auth.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hub == nil {
			c.JSON(503, gin.H{"error": "Skill sync watch not available (Redis not configured)"})
			return
		}

		tokenStr := c.Query("token")
		if tokenStr == "" {
			c.JSON(401, gin.H{"error": "token query parameter required"})
			return
		}
		claims, err := jwtService.ValidateToken(tokenStr)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid or expired token"})
			return
		}
		userID := claims.UserID

		agentIDStr := c.Query("agent_id")
		agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
		if err != nil || agentID <= 0 {
			c.JSON(400, gin.H{"error": "invalid agent_id"})
			return
		}

		conn, err := syncWsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Warn().Err(err).Msg("WatchSync: websocket upgrade failed")
			return
		}
		defer conn.Close()

		const (
			pingInterval = 30 * time.Second
			pongTimeout  = 60 * time.Second
		)

		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
			return nil
		})

		// Drain reads (required by gorilla/websocket to process pong frames)
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
		}()

		ch, unsub := hub.Subscribe(userID, agentID)
		defer unsub()

		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for {
			select {
			case event := <-ch:
				data, err := json.Marshal(event)
				if err != nil {
					log.Warn().Err(err).Msg("WatchSync: marshal error")
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}
