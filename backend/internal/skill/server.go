package skill

import (
	"context"
	"fmt"
	"log"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

type Server struct {
	sacv1.UnimplementedSkillServiceServer
	db          *bun.DB
	syncService *SyncService
}

func NewServer(db *bun.DB, syncService *SyncService) *Server {
	return &Server{db: db, syncService: syncService}
}

func (s *Server) ListSkills(ctx context.Context, _ *sacv1.Empty) (*sacv1.SkillListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	var skills []models.Skill
	err := s.db.NewSelect().
		Model(&skills).
		Where("is_official = ? OR created_by = ? OR is_public = ?", true, userID, true).
		Order("category ASC", "name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to retrieve skills", err)
	}

	return &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)}, nil
}

func (s *Server) GetSkill(ctx context.Context, req *sacv1.GetSkillRequest) (*sacv1.Skill, error) {
	var skill models.Skill
	err := s.db.NewSelect().Model(&skill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}
	return convert.SkillToProto(&skill), nil
}

func (s *Server) CreateSkill(ctx context.Context, req *sacv1.CreateSkillRequest) (*sacv1.Skill, error) {
	userID := ctxkeys.UserID(ctx)

	if req.Name == "" {
		return nil, grpcerr.BadRequest("name is required")
	}

	skill := models.Skill{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Category:    req.Category,
		Prompt:      req.Prompt,
		CommandName: req.CommandName,
		Parameters:  convert.SkillParametersFromProto(req.Parameters),
		IsPublic:    req.IsPublic,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsOfficial:  false,
	}

	if skill.CommandName == "" {
		skill.CommandName = SanitizeCommandName(skill.Name)
	}
	if skill.CommandName == "" {
		return nil, grpcerr.BadRequest("Cannot derive a valid command name from skill name")
	}

	exists, err := s.db.NewSelect().Model((*models.Skill)(nil)).
		Where("command_name = ?", skill.CommandName).
		Exists(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to check command name", err)
	}
	if exists {
		return nil, grpcerr.Conflict(fmt.Sprintf("Command name '/%s' is already taken", skill.CommandName))
	}

	_, err = s.db.NewInsert().Model(&skill).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create skill", err)
	}

	return convert.SkillToProto(&skill), nil
}

func (s *Server) UpdateSkill(ctx context.Context, req *sacv1.UpdateSkillByIdRequest) (*sacv1.Skill, error) {
	userID := ctxkeys.UserID(ctx)
	role := ctxkeys.Role(ctx)

	var existingSkill models.Skill
	err := s.db.NewSelect().Model(&existingSkill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}

	isAdmin := role == "admin"
	if existingSkill.IsOfficial && !isAdmin {
		return nil, grpcerr.Forbidden("Only admins can edit official skills")
	}
	if !existingSkill.IsOfficial && existingSkill.CreatedBy != userID {
		return nil, grpcerr.Forbidden("You don't have permission to update this skill")
	}

	var updateData models.Skill
	updateData.ID = req.Id
	updateData.UpdatedAt = time.Now()
	updateData.Version = existingSkill.Version + 1

	if req.Name != nil {
		updateData.Name = *req.Name
	}
	if req.Description != nil {
		updateData.Description = *req.Description
	}
	if req.Icon != nil {
		updateData.Icon = *req.Icon
	}
	if req.Category != nil {
		updateData.Category = *req.Category
	}
	if req.Prompt != nil {
		updateData.Prompt = *req.Prompt
	}
	if req.CommandName != nil {
		updateData.CommandName = *req.CommandName
	}
	if req.Parameters != nil {
		updateData.Parameters = convert.SkillParametersFromProto(req.Parameters)
	}
	if req.IsPublic != nil {
		updateData.IsPublic = *req.IsPublic
	}

	if updateData.CommandName == "" && updateData.Name != "" {
		updateData.CommandName = SanitizeCommandName(updateData.Name)
	}
	if updateData.CommandName == "" {
		updateData.CommandName = existingSkill.CommandName
	}

	if updateData.CommandName != existingSkill.CommandName {
		dup, dupErr := s.db.NewSelect().Model((*models.Skill)(nil)).
			Where("command_name = ? AND id != ?", updateData.CommandName, req.Id).
			Exists(ctx)
		if dupErr != nil {
			return nil, grpcerr.Internal("Failed to check command name", dupErr)
		}
		if dup {
			return nil, grpcerr.Conflict(fmt.Sprintf("Command name '/%s' is already taken", updateData.CommandName))
		}
	}

	_, err = s.db.NewUpdate().
		Model(&updateData).
		Column("name", "description", "icon", "category", "prompt", "command_name", "parameters", "is_public", "updated_at", "version").
		Where("id = ?", req.Id).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update skill", err)
	}

	var updatedSkill models.Skill
	err = s.db.NewSelect().Model(&updatedSkill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to reload skill after update", err)
	}

	return convert.SkillToProto(&updatedSkill), nil
}

func (s *Server) DeleteSkill(ctx context.Context, req *sacv1.GetSkillRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	var skill models.Skill
	err := s.db.NewSelect().Model(&skill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}

	if skill.CreatedBy != userID {
		return nil, grpcerr.Forbidden("You don't have permission to delete this skill")
	}
	if skill.IsOfficial {
		return nil, grpcerr.Forbidden("Cannot delete official skills")
	}

	var agentSkills []models.AgentSkill
	_ = s.db.NewSelect().Model(&agentSkills).Where("skill_id = ?", req.Id).Scan(ctx)

	_, err = s.db.NewDelete().Model(&skill).Where("id = ?", req.Id).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete skill", err)
	}

	if skill.CommandName != "" {
		go func() {
			bgCtx := context.Background()
			for _, as := range agentSkills {
				var agent models.Agent
				err := s.db.NewSelect().Model(&agent).Where("id = ?", as.AgentID).Scan(bgCtx)
				if err != nil {
					continue
				}
				userIDStr := fmt.Sprintf("%d", agent.CreatedBy)
				if err := s.syncService.RemoveSkillFromAgent(bgCtx, userIDStr, as.AgentID, skill.CommandName); err != nil {
					log.Printf("Warning: failed to remove skill /%s from agent %d: %v", skill.CommandName, as.AgentID, err)
				}
			}
		}()
	}

	return &sacv1.SuccessMessage{Message: "Skill deleted successfully"}, nil
}

func (s *Server) ForkSkill(ctx context.Context, req *sacv1.GetSkillRequest) (*sacv1.Skill, error) {
	userID := ctxkeys.UserID(ctx)

	var originalSkill models.Skill
	err := s.db.NewSelect().Model(&originalSkill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}

	if !originalSkill.IsPublic && !originalSkill.IsOfficial {
		return nil, grpcerr.Forbidden("This skill is not public")
	}

	forkedName := originalSkill.Name + " (Fork)"
	baseCmd := SanitizeCommandName(forkedName)
	cmdName := baseCmd

	for i := 2; i <= 100; i++ {
		exists, exErr := s.db.NewSelect().Model((*models.Skill)(nil)).
			Where("command_name = ?", cmdName).
			Exists(ctx)
		if exErr != nil {
			return nil, grpcerr.Internal("Failed to check command name", exErr)
		}
		if !exists {
			break
		}
		cmdName = fmt.Sprintf("%s-%d", baseCmd, i)
	}

	forkedSkill := models.Skill{
		Name:        forkedName,
		Description: originalSkill.Description,
		Icon:        originalSkill.Icon,
		Category:    originalSkill.Category,
		Prompt:      originalSkill.Prompt,
		CommandName: cmdName,
		Parameters:  originalSkill.Parameters,
		IsOfficial:  false,
		CreatedBy:   userID,
		IsPublic:    false,
		ForkedFrom:  &originalSkill.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = s.db.NewInsert().Model(&forkedSkill).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fork skill", err)
	}

	return convert.SkillToProto(&forkedSkill), nil
}

func (s *Server) ListPublicSkills(ctx context.Context, _ *sacv1.Empty) (*sacv1.SkillListResponse, error) {
	var skills []models.Skill
	err := s.db.NewSelect().
		Model(&skills).
		Where("is_public = ? OR is_official = ?", true, true).
		Relation("Creator").
		Order("category ASC", "name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to retrieve public skills", err)
	}

	return &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)}, nil
}
