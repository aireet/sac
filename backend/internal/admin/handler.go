package admin

import (
	"encoding/csv"
	"fmt"
	"time"

	"g.echo.tech/dev/sac/internal/container"
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

func (h *Handler) ExportConversations(c *gin.Context) {
	ctx := c.Request.Context()

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
