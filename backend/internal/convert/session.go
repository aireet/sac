package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func SessionToProto(m *models.Session) *sacv1.Session {
	return &sacv1.Session{
		Id:         m.ID,
		UserId:     m.UserID,
		AgentId:    m.AgentID,
		SessionId:  m.SessionID,
		PodName:    m.PodName,
		PodIp:      m.PodIP,
		Status:     string(m.Status),
		LastActive: timestamppb.New(m.LastActive),
		CreatedAt:  timestamppb.New(m.CreatedAt),
		UpdatedAt:  timestamppb.New(m.UpdatedAt),
	}
}

func SessionsToProto(ms []models.Session) []*sacv1.Session {
	out := make([]*sacv1.Session, len(ms))
	for i := range ms {
		out[i] = SessionToProto(&ms[i])
	}
	return out
}
