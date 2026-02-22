package history

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes registers JWT-protected conversation history query routes.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/conversations", h.listConversations)
	rg.GET("/conversations/sessions", h.listSessions)
	rg.GET("/conversations/export", h.ExportConversations)
}

// RegisterInternalRoutes registers internal routes (no JWT, Pod-internal calls).
func (h *Handler) RegisterInternalRoutes(rg *gin.RouterGroup) {
	rg.POST("/conversations/events", h.receiveEvents)
}

// --- Internal endpoint: receive hook events from Pods ---

func (h *Handler) receiveEvents(c *gin.Context) {
	req := &sacv1.EventsRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.UserId == "" || req.AgentId == "" || req.SessionId == "" {
		response.BadRequest(c, "user_id, agent_id, and session_id are required")
		return
	}

	if len(req.Messages) == 0 {
		protobind.OK(c, &sacv1.EventsResponse{Inserted: 0})
		return
	}

	userID, err := strconv.ParseInt(req.UserId, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid user_id")
		return
	}
	agentID, err := strconv.ParseInt(req.AgentId, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	// Validate user and agent exist
	var userExists bool
	userExists, err = h.db.NewSelect().TableExpr("users").Where("id = ?", userID).Exists(c.Request.Context())
	if err != nil || !userExists {
		response.BadRequest(c, "user not found")
		return
	}
	var agentExists bool
	agentExists, err = h.db.NewSelect().TableExpr("agents").Where("id = ?", agentID).Exists(c.Request.Context())
	if err != nil || !agentExists {
		response.BadRequest(c, "agent not found")
		return
	}

	// Build records
	records := make([]models.ConversationHistory, 0, len(req.Messages))
	for _, msg := range req.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		if msg.Content == "" {
			continue
		}

		ts := time.Now()
		if msg.Timestamp != "" {
			if parsed, e := time.Parse(time.RFC3339, msg.Timestamp); e == nil {
				ts = parsed
			}
		}

		records = append(records, models.ConversationHistory{
			UserID:      userID,
			AgentID:     agentID,
			SessionID:   req.SessionId,
			Role:        msg.Role,
			Content:     msg.Content,
			MessageUUID: msg.Uuid,
			Timestamp:   ts,
		})
	}

	if len(records) == 0 {
		protobind.OK(c, &sacv1.EventsResponse{Inserted: 0})
		return
	}

	_, err = h.db.NewInsert().Model(&records).Exec(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to insert conversation history", err)
		return
	}

	protobind.OK(c, &sacv1.EventsResponse{Inserted: int32(len(records))})
}

// --- Protected endpoint: query conversation history ---

func (h *Handler) listConversations(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	agentIDStr := c.Query("agent_id")
	if agentIDStr == "" {
		response.BadRequest(c, "agent_id is required")
		return
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, e := strconv.Atoi(l); e == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	query := h.db.NewSelect().
		Model((*models.ConversationHistory)(nil)).
		Where("user_id = ?", userID).
		Where("agent_id = ?", agentID).
		Limit(limit + 1) // fetch one extra to detect if there's a next page

	// Cursor-based pagination
	direction := "desc" // default: newest first
	if before := c.Query("before"); before != "" {
		if ts, e := time.Parse(time.RFC3339Nano, before); e == nil {
			query = query.Where("timestamp < ?", ts)
		}
	}
	if after := c.Query("after"); after != "" {
		if ts, e := time.Parse(time.RFC3339Nano, after); e == nil {
			query = query.Where("timestamp > ?", ts)
			direction = "asc"
		}
	}

	if direction == "asc" {
		query = query.OrderExpr("timestamp ASC")
	} else {
		query = query.OrderExpr("timestamp DESC")
	}

	// Optional session_id filter
	if sid := c.Query("session_id"); sid != "" {
		query = query.Where("session_id = ?", sid)
	}

	var histories []models.ConversationHistory
	err = query.Scan(c.Request.Context(), &histories)
	if err != nil {
		response.InternalError(c, "Failed to query conversation history", err)
		return
	}

	if histories == nil {
		histories = []models.ConversationHistory{}
	}

	hasMore := len(histories) > limit
	if hasMore {
		histories = histories[:limit]
	}

	// Always return in chronological order (ASC)
	if direction == "desc" {
		for i, j := 0, len(histories)-1; i < j; i, j = i+1, j-1 {
			histories[i], histories[j] = histories[j], histories[i]
		}
	}

	protobind.JSON(c, http.StatusOK, &sacv1.ConversationListResponse{
		Conversations: convert.ConversationHistoriesToProto(histories),
		Count:         int32(len(histories)),
		HasMore:       hasMore,
	})
}

func (h *Handler) listSessions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	agentIDStr := c.Query("agent_id")
	if agentIDStr == "" {
		response.BadRequest(c, "agent_id is required")
		return
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	type sessionRow struct {
		SessionID string    `bun:"session_id"`
		FirstAt   time.Time `bun:"first_at"`
		LastAt    time.Time `bun:"last_at"`
		Count     int       `bun:"count"`
	}

	var sessions []sessionRow
	err = h.db.NewSelect().
		TableExpr("conversation_histories").
		ColumnExpr("session_id").
		ColumnExpr("MIN(timestamp) AS first_at").
		ColumnExpr("MAX(timestamp) AS last_at").
		ColumnExpr("COUNT(*) AS count").
		Where("user_id = ?", userID).
		Where("agent_id = ?", agentID).
		GroupExpr("session_id").
		OrderExpr("last_at DESC").
		Limit(50).
		Scan(c.Request.Context(), &sessions)
	if err != nil {
		response.InternalError(c, "Failed to list sessions", err)
		return
	}

	if sessions == nil {
		sessions = []sessionRow{}
	}

	pbSessions := make([]*sacv1.SessionSummary, len(sessions))
	for i, s := range sessions {
		pbSessions[i] = &sacv1.SessionSummary{
			SessionId: s.SessionID,
			FirstAt:   timestamppb.New(s.FirstAt),
			LastAt:    timestamppb.New(s.LastAt),
			Count:     int32(s.Count),
		}
	}

	protobind.JSON(c, http.StatusOK, &sacv1.SessionListResponse{
		Sessions: pbSessions,
	})
}

func (h *Handler) ExportConversations(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	agentIDStr := c.Query("agent_id")
	if agentIDStr == "" {
		response.BadRequest(c, "agent_id is required")
		return
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	query := h.db.NewSelect().
		Model((*models.ConversationHistory)(nil)).
		Where("user_id = ?", userID).
		Where("agent_id = ?", agentID).
		OrderExpr("timestamp ASC")

	if sid := c.Query("session_id"); sid != "" {
		query = query.Where("session_id = ?", sid)
	}

	var histories []models.ConversationHistory
	err = query.Scan(c.Request.Context(), &histories)
	if err != nil {
		response.InternalError(c, "Failed to query conversation history", err)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=conversations_%s.csv", time.Now().Format("20060102_150405")))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"timestamp", "session_id", "role", "content"})
	for _, r := range histories {
		_ = w.Write([]string{
			r.Timestamp.Format(time.RFC3339),
			r.SessionID,
			r.Role,
			r.Content,
		})
	}
	w.Flush()
}
