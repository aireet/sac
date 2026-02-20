package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func GroupToProto(m *models.Group) *sacv1.Group {
	pb := &sacv1.Group{
		Id:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		OwnerId:          m.OwnerID,
		ClaudeMdTemplate: m.ClaudeMDTemplate,
		CreatedAt:        timestamppb.New(m.CreatedAt),
		UpdatedAt:        timestamppb.New(m.UpdatedAt),
	}
	if m.Owner != nil {
		pb.Owner = UserBriefToProto(m.Owner)
	}
	return pb
}

func GroupMemberToProto(m *models.GroupMember) *sacv1.GroupMember {
	pb := &sacv1.GroupMember{
		Id:        m.ID,
		GroupId:   m.GroupID,
		UserId:    m.UserID,
		Role:      m.Role,
		CreatedAt: timestamppb.New(m.CreatedAt),
	}
	if m.User != nil {
		pb.User = UserBriefToProto(m.User)
	}
	return pb
}

func GroupMembersToProto(ms []models.GroupMember) []*sacv1.GroupMember {
	out := make([]*sacv1.GroupMember, len(ms))
	for i := range ms {
		out[i] = GroupMemberToProto(&ms[i])
	}
	return out
}
