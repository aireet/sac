package convert

import (
	"g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func UserToProto(m *models.User) *sacv1.User {
	return &sacv1.User{
		Id:          m.ID,
		Username:    m.Username,
		Email:       m.Email,
		DisplayName: m.DisplayName,
		Role:        m.Role,
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
	}
}

func UserBriefToProto(m *models.User) *sacv1.UserBrief {
	if m == nil {
		return nil
	}
	return &sacv1.UserBrief{
		Id:          m.ID,
		Username:    m.Username,
		DisplayName: m.DisplayName,
	}
}
