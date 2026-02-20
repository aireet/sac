package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConversationHistoryToProto(m *models.ConversationHistory) *sacv1.ConversationMessage {
	return &sacv1.ConversationMessage{
		Id:          m.ID,
		UserId:      m.UserID,
		AgentId:     m.AgentID,
		SessionId:   m.SessionID,
		Role:        m.Role,
		Content:     m.Content,
		MessageUuid: m.MessageUUID,
		Timestamp:   timestamppb.New(m.Timestamp),
	}
}

func ConversationHistoriesToProto(ms []models.ConversationHistory) []*sacv1.ConversationMessage {
	out := make([]*sacv1.ConversationMessage, len(ms))
	for i := range ms {
		out[i] = ConversationHistoryToProto(&ms[i])
	}
	return out
}
