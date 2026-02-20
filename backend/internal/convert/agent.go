package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func AgentToProto(m *models.Agent) *sacv1.Agent {
	pb := &sacv1.Agent{
		Id:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		Icon:          m.Icon,
		Instructions:  m.Instructions,
		CreatedBy:     m.CreatedBy,
		CpuRequest:    m.CPURequest,
		CpuLimit:      m.CPULimit,
		MemoryRequest: m.MemoryRequest,
		MemoryLimit:   m.MemoryLimit,
		CreatedAt:     timestamppb.New(m.CreatedAt),
		UpdatedAt:     timestamppb.New(m.UpdatedAt),
	}
	if m.Config != nil {
		if s, err := structpb.NewStruct(map[string]any(m.Config)); err == nil {
			pb.Config = s
		}
	}
	for i := range m.InstalledSkills {
		pb.InstalledSkills = append(pb.InstalledSkills, AgentSkillToProto(&m.InstalledSkills[i]))
	}
	return pb
}

func AgentsToProto(ms []models.Agent) []*sacv1.Agent {
	out := make([]*sacv1.Agent, len(ms))
	for i := range ms {
		out[i] = AgentToProto(&ms[i])
	}
	return out
}

func AgentSkillToProto(m *models.AgentSkill) *sacv1.AgentSkill {
	pb := &sacv1.AgentSkill{
		Id:            m.ID,
		AgentId:       m.AgentID,
		SkillId:       m.SkillID,
		Order:         int32(m.Order),
		SyncedVersion: int32(m.SyncedVersion),
		CreatedAt:     timestamppb.New(m.CreatedAt),
	}
	if m.Skill != nil {
		pb.Skill = SkillToProto(m.Skill)
	}
	return pb
}
