package group

import (
	"context"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

// Server implements both GroupServiceServer and AdminGroupServiceServer.
type Server struct {
	sacv1.UnimplementedGroupServiceServer
	sacv1.UnimplementedAdminGroupServiceServer
	db *bun.DB
}

func NewServer(db *bun.DB) *Server {
	return &Server{db: db}
}

// --- GroupService (authenticated users) ---

func (s *Server) ListGroups(ctx context.Context, _ *sacv1.Empty) (*sacv1.GroupListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	var groups []models.Group
	err := s.db.NewSelect().Model(&groups).
		Where("g.id IN (SELECT group_id FROM group_members WHERE user_id = ?)", userID).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("g.name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list groups", err)
	}

	if len(groups) == 0 {
		return &sacv1.GroupListResponse{Groups: []*sacv1.GroupWithMemberCount{}}, nil
	}

	countMap := s.getMemberCountsServer(ctx, groupIDs(groups))
	result := make([]*sacv1.GroupWithMemberCount, len(groups))
	for i, g := range groups {
		result[i] = &sacv1.GroupWithMemberCount{
			Group:       convert.GroupToProto(&g),
			MemberCount: int32(countMap[g.ID]),
		}
	}

	return &sacv1.GroupListResponse{Groups: result}, nil
}

func (s *Server) GetGroup(ctx context.Context, req *sacv1.GetGroupRequest) (*sacv1.Group, error) {
	userID := ctxkeys.UserID(ctx)

	if !s.isMember(ctx, req.Id, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	var group models.Group
	err := s.db.NewSelect().Model(&group).
		Where("g.id = ?", req.Id).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Group not found")
	}

	return convert.GroupToProto(&group), nil
}

func (s *Server) ListMembers(ctx context.Context, req *sacv1.GetGroupRequest) (*sacv1.GroupMemberListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if !s.isMember(ctx, req.Id, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	return s.listMembersInternal(ctx, req.Id)
}

func (s *Server) GetTemplate(ctx context.Context, req *sacv1.GetGroupRequest) (*sacv1.GroupTemplateResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if !s.isMember(ctx, req.Id, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	var group models.Group
	err := s.db.NewSelect().Model(&group).
		Column("claude_md_template").
		Where("id = ?", req.Id).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Group not found")
	}

	return &sacv1.GroupTemplateResponse{ClaudeMdTemplate: group.ClaudeMDTemplate}, nil
}

func (s *Server) UpdateTemplate(ctx context.Context, req *sacv1.UpdateTemplateByGroupRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)
	role := ctxkeys.Role(ctx)

	if role != "admin" && !s.isGroupAdmin(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Only group admins or system admins can update the template")
	}

	_, err := s.db.NewUpdate().Model((*models.Group)(nil)).
		Set("claude_md_template = ?", req.ClaudeMdTemplate).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", req.GroupId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update template", err)
	}

	return &sacv1.SuccessMessage{Message: "Template updated"}, nil
}

// --- AdminGroupService ---

func (s *Server) ListAllGroups(ctx context.Context, _ *sacv1.Empty) (*sacv1.GroupListResponse, error) {
	var groups []models.Group
	err := s.db.NewSelect().Model(&groups).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("g.name ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list groups", err)
	}

	if len(groups) == 0 {
		return &sacv1.GroupListResponse{Groups: []*sacv1.GroupWithMemberCount{}}, nil
	}

	countMap := s.getMemberCountsServer(ctx, groupIDs(groups))
	result := make([]*sacv1.GroupWithMemberCount, len(groups))
	for i, g := range groups {
		result[i] = &sacv1.GroupWithMemberCount{
			Group:       convert.GroupToProto(&g),
			MemberCount: int32(countMap[g.ID]),
		}
	}

	return &sacv1.GroupListResponse{Groups: result}, nil
}

func (s *Server) CreateGroup(ctx context.Context, req *sacv1.CreateGroupRequest) (*sacv1.Group, error) {
	userID := ctxkeys.UserID(ctx)

	if req.Name == "" {
		return nil, grpcerr.BadRequest("name is required")
	}

	exists, err := s.db.NewSelect().Model((*models.Group)(nil)).
		Where("name = ?", req.Name).
		Exists(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to check group name", err)
	}
	if exists {
		return nil, grpcerr.Conflict("A group with this name already exists")
	}

	ownerID := userID
	if req.OwnerId != nil && *req.OwnerId > 0 {
		ownerID = *req.OwnerId
	}

	now := time.Now()
	group := &models.Group{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     ownerID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, grpcerr.Internal("Failed to start transaction", err)
	}
	defer tx.Rollback()

	_, err = tx.NewInsert().Model(group).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create group", err)
	}

	member := &models.GroupMember{
		GroupID:   group.ID,
		UserID:    ownerID,
		Role:      "admin",
		CreatedAt: now,
	}
	_, err = tx.NewInsert().Model(member).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to add owner as member", err)
	}

	quota := &models.GroupWorkspaceQuota{
		GroupID:      group.ID,
		MaxBytes:     1 << 30,
		MaxFileCount: 1000,
		UpdatedAt:    now,
	}
	_, err = tx.NewInsert().Model(quota).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create quota", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, grpcerr.Internal("Failed to commit", err)
	}

	return convert.GroupToProto(group), nil
}

func (s *Server) UpdateGroup(ctx context.Context, req *sacv1.UpdateGroupByIdRequest) (*sacv1.SuccessMessage, error) {
	q := s.db.NewUpdate().Model((*models.Group)(nil)).Where("id = ?", req.Id)
	if req.Name != nil {
		q = q.Set("name = ?", *req.Name)
	}
	if req.Description != nil {
		q = q.Set("description = ?", *req.Description)
	}
	if req.ClaudeMdTemplate != nil {
		q = q.Set("claude_md_template = ?", *req.ClaudeMdTemplate)
	}
	q = q.Set("updated_at = ?", time.Now())

	_, err := q.Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update group", err)
	}

	return &sacv1.SuccessMessage{Message: "Group updated"}, nil
}

func (s *Server) DeleteGroup(ctx context.Context, req *sacv1.GetGroupRequest) (*sacv1.SuccessMessage, error) {
	_, err := s.db.NewDelete().Model((*models.Group)(nil)).Where("id = ?", req.Id).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete group", err)
	}
	return &sacv1.SuccessMessage{Message: "Group deleted"}, nil
}

func (s *Server) ListMembersAdmin(ctx context.Context, req *sacv1.GetGroupRequest) (*sacv1.GroupMemberListResponse, error) {
	return s.listMembersInternal(ctx, req.Id)
}

func (s *Server) AddMember(ctx context.Context, req *sacv1.AddMemberByGroupRequest) (*sacv1.GroupMember, error) {
	if req.UserId == 0 {
		return nil, grpcerr.BadRequest("user_id is required")
	}
	if req.Role == "" {
		req.Role = "member"
	}

	var user models.User
	err := s.db.NewSelect().Model(&user).Where("id = ?", req.UserId).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("User not found")
	}

	if s.isMember(ctx, req.GroupId, req.UserId) {
		return nil, grpcerr.Conflict("User is already a member of this group")
	}

	member := &models.GroupMember{
		GroupID:   req.GroupId,
		UserID:    req.UserId,
		Role:      req.Role,
		CreatedAt: time.Now(),
	}

	_, err = s.db.NewInsert().Model(member).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to add member", err)
	}

	return convert.GroupMemberToProto(member), nil
}

func (s *Server) RemoveMember(ctx context.Context, req *sacv1.RemoveMemberRequest) (*sacv1.SuccessMessage, error) {
	var group models.Group
	if err := s.db.NewSelect().Model(&group).Where("id = ?", req.GroupId).Scan(ctx); err == nil {
		if group.OwnerID == req.UserId {
			return nil, grpcerr.BadRequest("Cannot remove the group owner")
		}
	}

	_, err := s.db.NewDelete().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", req.GroupId, req.UserId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to remove member", err)
	}

	return &sacv1.SuccessMessage{Message: "Member removed"}, nil
}

func (s *Server) UpdateMemberRole(ctx context.Context, req *sacv1.UpdateMemberRoleByGroupRequest) (*sacv1.SuccessMessage, error) {
	if req.Role == "" {
		return nil, grpcerr.BadRequest("role is required")
	}

	_, err := s.db.NewUpdate().Model((*models.GroupMember)(nil)).
		Set("role = ?", req.Role).
		Where("group_id = ? AND user_id = ?", req.GroupId, req.UserId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update member role", err)
	}

	return &sacv1.SuccessMessage{Message: "Member role updated"}, nil
}

func (s *Server) AdminUpdateTemplate(ctx context.Context, req *sacv1.UpdateTemplateByGroupRequest) (*sacv1.SuccessMessage, error) {
	_, err := s.db.NewUpdate().Model((*models.Group)(nil)).
		Set("claude_md_template = ?", req.ClaudeMdTemplate).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", req.GroupId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update template", err)
	}

	return &sacv1.SuccessMessage{Message: "Template updated"}, nil
}

// --- Helpers ---

func (s *Server) listMembersInternal(ctx context.Context, groupID int64) (*sacv1.GroupMemberListResponse, error) {
	var members []models.GroupMember
	err := s.db.NewSelect().Model(&members).
		Where("gm.group_id = ?", groupID).
		Relation("User", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("gm.created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list members", err)
	}

	return &sacv1.GroupMemberListResponse{Members: convert.GroupMembersToProto(members)}, nil
}

func (s *Server) isMember(ctx context.Context, groupID, userID int64) bool {
	exists, _ := s.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Exists(ctx)
	return exists
}

func (s *Server) isGroupAdmin(ctx context.Context, groupID, userID int64) bool {
	exists, _ := s.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ? AND role = 'admin'", groupID, userID).
		Exists(ctx)
	return exists
}

func (s *Server) getMemberCountsServer(ctx context.Context, ids []int64) map[int64]int {
	type countResult struct {
		GroupID int64 `bun:"group_id"`
		Count   int   `bun:"count"`
	}
	var counts []countResult
	_ = s.db.NewSelect().
		TableExpr("group_members").
		Column("group_id").
		ColumnExpr("COUNT(*) AS count").
		Where("group_id IN (?)", bun.In(ids)).
		Group("group_id").
		Scan(ctx, &counts)

	countMap := make(map[int64]int)
	for _, cnt := range counts {
		countMap[cnt.GroupID] = cnt.Count
	}
	return countMap
}
