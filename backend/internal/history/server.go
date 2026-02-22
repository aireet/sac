package history

import (
	"context"
	"strconv"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	sacv1.UnimplementedHistoryServiceServer
	db *bun.DB
}

func NewServer(db *bun.DB) *Server {
	return &Server{db: db}
}

func (s *Server) ReceiveEvents(ctx context.Context, req *sacv1.EventsRequest) (*sacv1.EventsResponse, error) {
	if req.UserId == "" || req.AgentId == "" || req.SessionId == "" {
		return nil, grpcerr.BadRequest("user_id, agent_id, and session_id are required")
	}

	if len(req.Messages) == 0 {
		return &sacv1.EventsResponse{Inserted: 0}, nil
	}

	userID, err := strconv.ParseInt(req.UserId, 10, 64)
	if err != nil {
		return nil, grpcerr.BadRequest("invalid user_id")
	}
	agentID, err := strconv.ParseInt(req.AgentId, 10, 64)
	if err != nil {
		return nil, grpcerr.BadRequest("invalid agent_id")
	}

	userExists, err := s.db.NewSelect().TableExpr("users").Where("id = ?", userID).Exists(ctx)
	if err != nil || !userExists {
		return nil, grpcerr.BadRequest("user not found")
	}
	agentExists, err := s.db.NewSelect().TableExpr("agents").Where("id = ?", agentID).Exists(ctx)
	if err != nil || !agentExists {
		return nil, grpcerr.BadRequest("agent not found")
	}

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
		return &sacv1.EventsResponse{Inserted: 0}, nil
	}

	_, err = s.db.NewInsert().Model(&records).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to insert conversation history", err)
	}

	return &sacv1.EventsResponse{Inserted: int32(len(records))}, nil
}

func (s *Server) ListConversations(ctx context.Context, req *sacv1.ListConversationsRequest) (*sacv1.ConversationListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if req.AgentId == 0 {
		return nil, grpcerr.BadRequest("agent_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	query := s.db.NewSelect().
		Model((*models.ConversationHistory)(nil)).
		Where("user_id = ?", userID).
		Where("agent_id = ?", req.AgentId).
		Limit(limit + 1)

	direction := "desc"
	if req.Before != "" {
		if ts, e := time.Parse(time.RFC3339Nano, req.Before); e == nil {
			query = query.Where("timestamp < ?", ts)
		}
	}
	if req.After != "" {
		if ts, e := time.Parse(time.RFC3339Nano, req.After); e == nil {
			query = query.Where("timestamp > ?", ts)
			direction = "asc"
		}
	}

	if direction == "asc" {
		query = query.OrderExpr("timestamp ASC")
	} else {
		query = query.OrderExpr("timestamp DESC")
	}

	if req.SessionId != "" {
		query = query.Where("session_id = ?", req.SessionId)
	}

	var histories []models.ConversationHistory
	err := query.Scan(ctx, &histories)
	if err != nil {
		return nil, grpcerr.Internal("Failed to query conversation history", err)
	}

	if histories == nil {
		histories = []models.ConversationHistory{}
	}

	hasMore := len(histories) > limit
	if hasMore {
		histories = histories[:limit]
	}

	if direction == "desc" {
		for i, j := 0, len(histories)-1; i < j; i, j = i+1, j-1 {
			histories[i], histories[j] = histories[j], histories[i]
		}
	}

	return &sacv1.ConversationListResponse{
		Conversations: convert.ConversationHistoriesToProto(histories),
		Count:         int32(len(histories)),
		HasMore:       hasMore,
	}, nil
}

func (s *Server) ListSessions(ctx context.Context, req *sacv1.ListSessionsRequest) (*sacv1.SessionListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if req.AgentId == 0 {
		return nil, grpcerr.BadRequest("agent_id is required")
	}

	type sessionRow struct {
		SessionID string    `bun:"session_id"`
		FirstAt   time.Time `bun:"first_at"`
		LastAt    time.Time `bun:"last_at"`
		Count     int       `bun:"count"`
	}

	var sessions []sessionRow
	err := s.db.NewSelect().
		TableExpr("conversation_histories").
		ColumnExpr("session_id").
		ColumnExpr("MIN(timestamp) AS first_at").
		ColumnExpr("MAX(timestamp) AS last_at").
		ColumnExpr("COUNT(*) AS count").
		Where("user_id = ?", userID).
		Where("agent_id = ?", req.AgentId).
		GroupExpr("session_id").
		OrderExpr("last_at DESC").
		Limit(50).
		Scan(ctx, &sessions)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list sessions", err)
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

	return &sacv1.SessionListResponse{Sessions: pbSessions}, nil
}
