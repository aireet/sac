package skill

import (
	"context"
	"fmt"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/rs/zerolog/log"
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
		Relation("Files").
		Where(`is_official = ? OR created_by = ? OR is_public = ?
			OR group_id IN (SELECT group_id FROM group_members WHERE user_id = ?)`,
			true, userID, true, userID).
		Order("category ASC", "name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to retrieve skills", err)
	}

	return &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)}, nil
}

func (s *Server) GetSkill(ctx context.Context, req *sacv1.GetSkillRequest) (*sacv1.Skill, error) {
	var skill models.Skill
	err := s.db.NewSelect().Model(&skill).Relation("Files").Where("sk.id = ?", req.Id).Scan(ctx)
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
		Frontmatter: convert.FrontmatterFromProto(req.Frontmatter),
		IsPublic:    req.IsPublic,
		GroupID:     req.GroupId,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsOfficial:  false,
	}

	// Validate group membership if group_id is set
	if skill.GroupID != nil && *skill.GroupID > 0 {
		if !s.isGroupMember(ctx, *skill.GroupID, userID) {
			return nil, grpcerr.Forbidden("You are not a member of this group")
		}
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

	// Dynamic columns: only update fields that were actually provided
	columns := []string{"updated_at", "version"}

	if req.Name != nil {
		updateData.Name = *req.Name
		columns = append(columns, "name")
	}
	if req.Description != nil {
		updateData.Description = *req.Description
		columns = append(columns, "description")
	}
	if req.Icon != nil {
		updateData.Icon = *req.Icon
		columns = append(columns, "icon")
	}
	if req.Category != nil {
		updateData.Category = *req.Category
		columns = append(columns, "category")
	}
	if req.Prompt != nil {
		updateData.Prompt = *req.Prompt
		columns = append(columns, "prompt")
	}
	if req.CommandName != nil {
		updateData.CommandName = *req.CommandName
		columns = append(columns, "command_name")
	}
	if req.Parameters != nil {
		updateData.Parameters = convert.SkillParametersFromProto(req.Parameters)
		columns = append(columns, "parameters")
	}
	if req.IsPublic != nil {
		updateData.IsPublic = *req.IsPublic
		columns = append(columns, "is_public")
	}
	if req.Frontmatter != nil {
		updateData.Frontmatter = convert.FrontmatterFromProto(req.Frontmatter)
		columns = append(columns, "frontmatter")
	}
	if req.GroupId != nil {
		if *req.GroupId > 0 && !s.isGroupMember(ctx, *req.GroupId, userID) {
			return nil, grpcerr.Forbidden("You are not a member of this group")
		}
		updateData.GroupID = req.GroupId
		columns = append(columns, "group_id")
	}

	// Mutual exclusion: is_public and group_id cannot both be set
	if req.IsPublic != nil && *req.IsPublic {
		updateData.GroupID = nil // clear group when going public
		columns = append(columns, "group_id")
	}
	if req.GroupId != nil && *req.GroupId > 0 {
		updateData.IsPublic = false // clear public when assigning to group
		columns = append(columns, "is_public")
	}

	if updateData.CommandName == "" && updateData.Name != "" {
		updateData.CommandName = SanitizeCommandName(updateData.Name)
		columns = append(columns, "command_name")
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
		Column(columns...).
		Where("id = ?", req.Id).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update skill", err)
	}

	// Rebuild bundle.tar when content changed (prompt or frontmatter)
	if req.Prompt != nil || req.Frontmatter != nil {
		if err := s.syncService.RebuildSkillBundle(ctx, req.Id); err != nil {
			log.Warn().Err(err).Int64("skill_id", req.Id).Msg("failed to rebuild skill bundle after update")
		}
	}

	var updatedSkill models.Skill
	err = s.db.NewSelect().Model(&updatedSkill).Relation("Files").Where("sk.id = ?", req.Id).Scan(ctx)
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

	// Remove agent_skills rows — agents can no longer see this skill.
	// Pod file cleanup is handled by the periodic sync cronjob.
	_, _ = s.db.NewDelete().Model((*models.AgentSkill)(nil)).Where("skill_id = ?", req.Id).Exec(ctx)

	_, err = s.db.NewDelete().Model(&skill).Where("id = ?", req.Id).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete skill", err)
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
		Frontmatter: originalSkill.Frontmatter,
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

	// Copy attached files from original skill
	fileCount, _ := s.db.NewSelect().Model((*models.SkillFile)(nil)).Where("skill_id = ?", req.Id).Count(ctx)
	if fileCount > 0 {
		go s.syncService.CopySkillFiles(context.Background(), originalSkill.ID, forkedSkill.ID)
	}

	return convert.SkillToProto(&forkedSkill), nil
}

func (s *Server) ListPublicSkills(ctx context.Context, _ *sacv1.Empty) (*sacv1.SkillListResponse, error) {
	var skills []models.Skill
	err := s.db.NewSelect().
		Model(&skills).
		Relation("Files").
		Where("is_public = ? OR is_official = ?", true, true).
		Relation("Creator").
		Order("category ASC", "name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to retrieve public skills", err)
	}

	return &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)}, nil
}

func (s *Server) ListGroupSkills(ctx context.Context, req *sacv1.ListGroupSkillsRequest) (*sacv1.SkillListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if req.GroupId == 0 {
		return nil, grpcerr.BadRequest("group_id is required")
	}
	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	var skills []models.Skill
	err := s.db.NewSelect().
		Model(&skills).
		Relation("Files").
		Relation("Creator").
		Where("group_id = ?", req.GroupId).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to retrieve group skills", err)
	}

	return &sacv1.SkillListResponse{Skills: convert.SkillsToProto(skills)}, nil
}

func (s *Server) ShareSkillToGroup(ctx context.Context, req *sacv1.ShareSkillToGroupRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	if req.Id == 0 || req.GroupId == 0 {
		return nil, grpcerr.BadRequest("skill id and group_id are required")
	}

	var skill models.Skill
	err := s.db.NewSelect().Model(&skill).Where("id = ?", req.Id).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Skill not found", err)
	}

	if skill.CreatedBy != userID {
		return nil, grpcerr.Forbidden("You can only share your own skills")
	}

	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	groupID := req.GroupId
	_, err = s.db.NewUpdate().
		Model((*models.Skill)(nil)).
		Set("group_id = ?", groupID).
		Set("is_public = ?", false).
		Set("updated_at = ?", time.Now()).
		Set("version = version + 1").
		Where("id = ?", req.Id).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to share skill to group", err)
	}

	return &sacv1.SuccessMessage{Message: "Skill shared to group"}, nil
}

func (s *Server) isGroupMember(ctx context.Context, groupID, userID int64) bool {
	exists, _ := s.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Exists(ctx)
	return exists
}
