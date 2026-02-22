package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"github.com/uptrace/bun"
)

type Server struct {
	sacv1.UnimplementedAgentServiceServer
	db               *bun.DB
	containerManager *container.Manager
	syncService      *skill.SyncService
	settingsService  *admin.SettingsService
}

func NewServer(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService, settingsService *admin.SettingsService) *Server {
	return &Server{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
		settingsService:  settingsService,
	}
}

func (s *Server) ListAgents(ctx context.Context, _ *sacv1.Empty) (*sacv1.AgentListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	agents := make([]models.Agent, 0)
	err := s.db.NewSelect().
		Model(&agents).
		Where("created_by = ?", userID).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch agents", err)
	}

	return &sacv1.AgentListResponse{Agents: convert.AgentsToProto(agents)}, nil
}

func (s *Server) GetAgent(ctx context.Context, req *sacv1.GetAgentRequest) (*sacv1.Agent, error) {
	userID := ctxkeys.UserID(ctx)

	var agent models.Agent
	err := s.db.NewSelect().
		Model(&agent).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	return convert.AgentToProto(&agent), nil
}

func (s *Server) CreateAgent(ctx context.Context, req *sacv1.CreateAgentRequest) (*sacv1.Agent, error) {
	userID := ctxkeys.UserID(ctx)

	count, err := s.db.NewSelect().
		Model((*models.Agent)(nil)).
		Where("created_by = ?", userID).
		Count(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to check agent count", err)
	}

	maxAgents, _ := s.settingsService.GetMaxAgents(ctx, userID)
	if count >= maxAgents {
		return nil, grpcerr.BadRequest(fmt.Sprintf("Maximum agents limit reached, you can only create up to %d agents", maxAgents))
	}

	if req.Name == "" {
		return nil, grpcerr.BadRequest("name is required")
	}

	agent := &models.Agent{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Instructions: req.Instructions,
		CreatedBy:    userID,
	}
	if req.Config != nil {
		agent.Config = req.Config.AsMap()
	}

	_, err = s.db.NewInsert().Model(agent).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create agent", err)
	}

	return convert.AgentToProto(agent), nil
}

func (s *Server) UpdateAgent(ctx context.Context, req *sacv1.UpdateAgentByIdRequest) (*sacv1.Agent, error) {
	userID := ctxkeys.UserID(ctx)

	var existing models.Agent
	err := s.db.NewSelect().
		Model(&existing).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	q := s.db.NewUpdate().Model(&models.Agent{}).Where("id = ?", req.Id)
	if req.Name != nil {
		q = q.Set("name = ?", *req.Name)
	}
	if req.Description != nil {
		q = q.Set("description = ?", *req.Description)
	}
	if req.Icon != nil {
		q = q.Set("icon = ?", *req.Icon)
	}
	if req.Instructions != nil {
		q = q.Set("instructions = ?", *req.Instructions)
	}
	if req.Config != nil {
		q = q.Set("config = ?", models.AgentConfig(req.Config.AsMap()))
	}
	q = q.Set("updated_at = ?", time.Now())

	_, err = q.Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update agent", err)
	}

	// If config changed, delete existing StatefulSet so it gets recreated with new env vars
	if req.Config != nil {
		userIDStr := fmt.Sprintf("%d", userID)

		_, _ = s.db.NewUpdate().
			Model((*models.Session)(nil)).
			Set("status = ?", models.SessionStatusDeleted).
			Set("updated_at = ?", time.Now()).
			Where("agent_id = ?", req.Id).
			Where("user_id = ?", userID).
			Where("status IN (?)", bun.In([]string{string(models.SessionStatusRunning), string(models.SessionStatusCreating), string(models.SessionStatusIdle)})).
			Exec(ctx)

		if err := s.containerManager.DeleteStatefulSet(ctx, userIDStr, req.Id); err != nil {
			log.Printf("Note: no existing StatefulSet to delete for agent %d: %v", req.Id, err)
		}
	}

	err = s.db.NewSelect().Model(&existing).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch updated agent", err)
	}

	return convert.AgentToProto(&existing), nil
}

func (s *Server) DeleteAgent(ctx context.Context, req *sacv1.GetAgentRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)
	userIDStr := fmt.Sprintf("%d", userID)

	res, err := s.db.NewDelete().
		Model((*models.Agent)(nil)).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete agent", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, grpcerr.NotFound("Agent not found")
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", req.Id).
		Where("user_id = ?", userID).
		Exec(ctx)

	if err := s.containerManager.DeleteStatefulSet(ctx, userIDStr, req.Id); err != nil {
		log.Printf("Warning: failed to delete StatefulSet for agent %d: %v", req.Id, err)
	}

	return &sacv1.SuccessMessage{Message: "Agent deleted successfully"}, nil
}

func (s *Server) RestartAgent(ctx context.Context, req *sacv1.GetAgentRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)
	userIDStr := fmt.Sprintf("%d", userID)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", req.Id).
		Where("user_id = ?", userID).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	if err := s.containerManager.DeleteStatefulSet(ctx, userIDStr, req.Id); err != nil {
		log.Printf("Failed to delete StatefulSet for agent %d: %v", req.Id, err)
		return nil, grpcerr.Internal("Failed to restart agent", err)
	}

	log.Printf("Restarted agent %d (deleted StatefulSet) for user %s", req.Id, userIDStr)
	return &sacv1.SuccessMessage{Message: "Agent is restarting"}, nil
}

func (s *Server) InstallSkill(ctx context.Context, req *sacv1.InstallSkillByAgentRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	if req.SkillId == 0 {
		return nil, grpcerr.BadRequest("skill_id is required")
	}

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.AgentId, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	var sk models.Skill
	err = s.db.NewSelect().Model(&sk).Where("id = ?", req.SkillId).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}

	var maxOrder int
	_ = s.db.NewSelect().
		Model((*models.AgentSkill)(nil)).
		Column("order").
		Where("agent_id = ?", req.AgentId).
		Order("order DESC").
		Limit(1).
		Scan(ctx, &maxOrder)

	agentSkill := &models.AgentSkill{
		AgentID: req.AgentId,
		SkillID: req.SkillId,
		Order:   maxOrder + 1,
	}

	_, err = s.db.NewInsert().
		Model(agentSkill).
		On("CONFLICT (agent_id, skill_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to install skill", err)
	}

	go func() {
		bgCtx := context.Background()
		userIDStr := fmt.Sprintf("%d", userID)
		if err := s.syncService.SyncSkillToAgent(bgCtx, userIDStr, req.AgentId, &sk); err != nil {
			log.Printf("Warning: failed to sync skill /%s to agent %d: %v", sk.CommandName, req.AgentId, err)
		}
	}()

	return &sacv1.SuccessMessage{Message: "Skill installed successfully"}, nil
}

func (s *Server) UninstallSkill(ctx context.Context, req *sacv1.UninstallSkillRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.AgentId, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	var sk models.Skill
	_ = s.db.NewSelect().Model(&sk).Where("id = ?", req.SkillId).Scan(ctx)

	_, err = s.db.NewDelete().
		Model((*models.AgentSkill)(nil)).
		Where("agent_id = ? AND skill_id = ?", req.AgentId, req.SkillId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to uninstall skill", err)
	}

	if sk.CommandName != "" {
		go func() {
			bgCtx := context.Background()
			userIDStr := fmt.Sprintf("%d", userID)
			if err := s.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, req.AgentId, sk.CommandName); err != nil {
				log.Printf("Warning: failed to remove skill /%s from agent %d: %v", sk.CommandName, req.AgentId, err)
			}
		}()
	}

	return &sacv1.SuccessMessage{Message: "Skill uninstalled successfully"}, nil
}

func (s *Server) SyncSkills(ctx context.Context, req *sacv1.GetAgentRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	userIDStr := fmt.Sprintf("%d", userID)
	if err := s.syncService.SyncAllSkillsToAgent(ctx, userIDStr, req.Id); err != nil {
		log.Printf("Failed to sync skills for agent %d: %v", req.Id, err)
		return nil, grpcerr.Internal("Failed to sync skills", err)
	}

	return &sacv1.SuccessMessage{Message: "Skills synced successfully"}, nil
}

func (s *Server) GetAgentStatuses(ctx context.Context, _ *sacv1.Empty) (*sacv1.AgentStatusListResponse, error) {
	userID := ctxkeys.UserID(ctx)
	userIDStr := fmt.Sprintf("%d", userID)

	var agentIDs []int64
	err := s.db.NewSelect().
		Model((*models.Agent)(nil)).
		Column("id").
		Where("created_by = ?", userID).
		Scan(ctx, &agentIDs)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch agents", err)
	}

	statuses := make([]*sacv1.AgentStatus, 0, len(agentIDs))
	for _, aid := range agentIDs {
		info := s.containerManager.GetStatefulSetPodInfo(ctx, userIDStr, aid)
		statuses = append(statuses, &sacv1.AgentStatus{
			AgentId:       aid,
			PodName:       info.PodName,
			Status:        info.Status,
			RestartCount:  info.RestartCount,
			CpuRequest:    info.CPURequest,
			CpuLimit:      info.CPULimit,
			MemoryRequest: info.MemoryRequest,
			MemoryLimit:   info.MemoryLimit,
		})
	}

	return &sacv1.AgentStatusListResponse{Statuses: statuses}, nil
}

func (s *Server) PreviewClaudeMD(ctx context.Context, req *sacv1.GetAgentRequest) (*sacv1.ClaudeMDPreview, error) {
	userID := ctxkeys.UserID(ctx)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.Id, userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	sysInstructions := s.settingsService.GetAgentSystemInstructions(ctx)
	groupTemplates := s.getGroupTemplates(ctx, userID)

	var readonlyParts []string
	if sysInstructions != "" {
		readonlyParts = append(readonlyParts, sysInstructions)
	}
	readonlyParts = append(readonlyParts, groupTemplates...)

	return &sacv1.ClaudeMDPreview{
		Readonly:     strings.Join(readonlyParts, "\n\n---\n\n"),
		Instructions: agent.Instructions,
	}, nil
}

func (s *Server) getGroupTemplates(ctx context.Context, userID int64) []string {
	var templates []string
	err := s.db.NewSelect().
		TableExpr("groups AS g").
		ColumnExpr("g.claude_md_template").
		Join("JOIN group_members AS gm ON gm.group_id = g.id").
		Where("gm.user_id = ?", userID).
		Where("g.claude_md_template != ''").
		OrderExpr("g.name ASC").
		Scan(ctx, &templates)
	if err != nil {
		log.Printf("Warning: failed to get group templates for user %d: %v", userID, err)
		return nil
	}
	return templates
}
