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
		GroupId:     m.GroupID,
		Version:     int32(m.Version),
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
		Frontmatter: FrontmatterToProto(&m.Frontmatter),
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
	for _, f := range m.Files {
		pb.Files = append(pb.Files, SkillFileToProto(&f))
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

func FrontmatterToProto(f *models.SkillFrontmatter) *sacv1.SkillFrontmatter {
	if f == nil {
		return nil
	}
	pb := &sacv1.SkillFrontmatter{
		AllowedTools:           f.AllowedTools,
		Model:                  f.Model,
		Context:                f.Context,
		Agent:                  f.Agent,
		DisableModelInvocation: f.DisableModelInvocation,
		ArgumentHint:           f.ArgumentHint,
		UserInvocable:          f.UserInvocable,
	}
	return pb
}

func FrontmatterFromProto(pb *sacv1.SkillFrontmatter) models.SkillFrontmatter {
	if pb == nil {
		return models.SkillFrontmatter{}
	}
	return models.SkillFrontmatter{
		AllowedTools:           pb.AllowedTools,
		Model:                  pb.Model,
		Context:                pb.Context,
		Agent:                  pb.Agent,
		DisableModelInvocation: pb.DisableModelInvocation,
		ArgumentHint:           pb.ArgumentHint,
		UserInvocable:          pb.UserInvocable,
	}
}

func SkillFileToProto(f *models.SkillFile) *sacv1.SkillFile {
	return &sacv1.SkillFile{
		Id:          f.ID,
		SkillId:     f.SkillID,
		Filepath:    f.Filepath,
		Size:        f.Size,
		ContentType: f.ContentType,
		CreatedAt:   timestamppb.New(f.CreatedAt),
	}
}

func SkillFilesToProto(fs []models.SkillFile) []*sacv1.SkillFile {
	out := make([]*sacv1.SkillFile, len(fs))
	for i := range fs {
		out[i] = SkillFileToProto(&fs[i])
	}
	return out
}
