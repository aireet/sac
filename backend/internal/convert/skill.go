package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func SkillToProto(m *models.Skill) *sacv1.Skill {
	pb := &sacv1.Skill{
		Id:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Icon:        m.Icon,
		Category:    m.Category,
		Prompt:      m.Prompt,
		CommandName: m.CommandName,
		IsOfficial:  m.IsOfficial,
		CreatedBy:   m.CreatedBy,
		IsPublic:    m.IsPublic,
		ForkedFrom:  m.ForkedFrom,
		Version:     int32(m.Version),
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
	}
	for _, p := range m.Parameters {
		pb.Parameters = append(pb.Parameters, &sacv1.SkillParameter{
			Name:         p.Name,
			Label:        p.Label,
			Type:         p.Type,
			Required:     p.Required,
			DefaultValue: p.DefaultValue,
			Options:      p.Options,
		})
	}
	if m.Creator != nil {
		pb.Creator = UserBriefToProto(m.Creator)
	}
	return pb
}

func SkillsToProto(ms []models.Skill) []*sacv1.Skill {
	out := make([]*sacv1.Skill, len(ms))
	for i := range ms {
		out[i] = SkillToProto(&ms[i])
	}
	return out
}

func SkillParametersFromProto(params []*sacv1.SkillParameter) models.SkillParameters {
	if len(params) == 0 {
		return nil
	}
	out := make(models.SkillParameters, len(params))
	for i, p := range params {
		out[i] = models.SkillParameter{
			Name:         p.Name,
			Label:        p.Label,
			Type:         p.Type,
			Required:     p.Required,
			DefaultValue: p.DefaultValue,
			Options:      p.Options,
		}
	}
	return out
}
