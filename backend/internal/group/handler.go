package group

import (
	"context"
	"strconv"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

// Handler serves group HTTP endpoints.
type Handler struct {
	db *bun.DB
}

// NewHandler creates a new group handler.
func NewHandler(db *bun.DB) *Handler {
	return &Handler{db: db}
}

// RegisterRoutes registers read-only group routes for authenticated users.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/groups")
	{
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.GET("/:id/members", h.ListMembers)
		g.GET("/:id/template", h.GetTemplate)
		g.PUT("/:id/template", h.UpdateTemplate)
	}
}

// RegisterAdminRoutes registers group management routes on the admin router group.
// These routes are protected by AdminMiddleware from the admin package.
func (h *Handler) RegisterAdminRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/groups")
	{
		g.GET("", h.ListAll)
		g.POST("", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
		g.GET("/:id/members", h.ListMembersAdmin)
		g.POST("/:id/members", h.AddMember)
		g.DELETE("/:id/members/:userId", h.RemoveMember)
		g.PUT("/:id/members/:userId", h.UpdateMemberRole)
		g.PUT("/:id/template", h.UpdateTemplate)
	}
}

// List returns all groups the current user belongs to.
func (h *Handler) List(c *gin.Context) {
	userID := c.GetInt64("userID")
	ctx := context.Background()

	var groups []models.Group
	err := h.db.NewSelect().Model(&groups).
		Where("g.id IN (SELECT group_id FROM group_members WHERE user_id = ?)", userID).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("g.name ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list groups", err)
		return
	}

	if len(groups) == 0 {
		protobind.OK(c, &sacv1.GroupListResponse{Groups: []*sacv1.GroupWithMemberCount{}})
		return
	}

	countMap := h.getMemberCounts(ctx, groupIDs(groups))

	result := make([]*sacv1.GroupWithMemberCount, len(groups))
	for i, g := range groups {
		result[i] = &sacv1.GroupWithMemberCount{
			Group:       convert.GroupToProto(&g),
			MemberCount: int32(countMap[g.ID]),
		}
	}

	protobind.OK(c, &sacv1.GroupListResponse{Groups: result})
}

// ListAll returns all groups with member counts (admin-only endpoint).
func (h *Handler) ListAll(c *gin.Context) {
	ctx := context.Background()

	var groups []models.Group
	err := h.db.NewSelect().Model(&groups).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("g.name ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list groups", err)
		return
	}

	if len(groups) == 0 {
		protobind.OK(c, &sacv1.GroupListResponse{Groups: []*sacv1.GroupWithMemberCount{}})
		return
	}

	countMap := h.getMemberCounts(ctx, groupIDs(groups))

	result := make([]*sacv1.GroupWithMemberCount, len(groups))
	for i, g := range groups {
		result[i] = &sacv1.GroupWithMemberCount{
			Group:       convert.GroupToProto(&g),
			MemberCount: int32(countMap[g.ID]),
		}
	}

	protobind.OK(c, &sacv1.GroupListResponse{Groups: result})
}

// Create creates a new group (admin-only). The admin becomes owner and admin member.
func (h *Handler) Create(c *gin.Context) {
	userID := c.GetInt64("userID")

	req := &sacv1.CreateGroupRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Name == "" {
		response.BadRequest(c, "name is required")
		return
	}

	ctx := context.Background()

	// Check if group name already exists
	exists, err := h.db.NewSelect().Model((*models.Group)(nil)).
		Where("name = ?", req.Name).
		Exists(ctx)
	if err != nil {
		response.InternalError(c, "Failed to check group name", err)
		return
	}
	if exists {
		response.Conflict(c, "A group with this name already exists")
		return
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

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		response.InternalError(c, "Failed to start transaction", err)
		return
	}
	defer tx.Rollback()

	_, err = tx.NewInsert().Model(group).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create group", err)
		return
	}

	// Add owner as admin member
	member := &models.GroupMember{
		GroupID:   group.ID,
		UserID:    ownerID,
		Role:      "admin",
		CreatedAt: now,
	}
	_, err = tx.NewInsert().Model(member).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to add owner as member", err)
		return
	}

	// Create default quota
	quota := &models.GroupWorkspaceQuota{
		GroupID:      group.ID,
		MaxBytes:     1 << 30, // 1GB
		MaxFileCount: 1000,
		UpdatedAt:    now,
	}
	_, err = tx.NewInsert().Model(quota).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create quota", err)
		return
	}

	if err := tx.Commit(); err != nil {
		response.InternalError(c, "Failed to commit", err)
		return
	}

	protobind.Created(c, convert.GroupToProto(group))
}

// Get returns a single group by ID (must be a member).
func (h *Handler) Get(c *gin.Context) {
	userID := c.GetInt64("userID")
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	ctx := context.Background()

	if !h.isMember(ctx, groupID, userID) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	var group models.Group
	err := h.db.NewSelect().Model(&group).
		Where("g.id = ?", groupID).
		Relation("Owner", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		Scan(ctx)
	if err != nil {
		response.NotFound(c, "Group not found")
		return
	}

	protobind.OK(c, convert.GroupToProto(&group))
}

// Update updates a group (admin-only endpoint).
func (h *Handler) Update(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	req := &sacv1.UpdateGroupRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	ctx := context.Background()

	q := h.db.NewUpdate().Model((*models.Group)(nil)).Where("id = ?", groupID)
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
		response.InternalError(c, "Failed to update group", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Group updated"})
}

// Delete deletes a group (admin-only endpoint).
func (h *Handler) Delete(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	ctx := context.Background()

	_, err := h.db.NewDelete().Model((*models.Group)(nil)).Where("id = ?", groupID).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to delete group", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Group deleted"})
}

// ListMembers returns all members of a group.
func (h *Handler) ListMembers(c *gin.Context) {
	userID := c.GetInt64("userID")
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	ctx := context.Background()

	if !h.isMember(ctx, groupID, userID) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	var members []models.GroupMember
	err := h.db.NewSelect().Model(&members).
		Where("gm.group_id = ?", groupID).
		Relation("User", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("gm.created_at ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list members", err)
		return
	}

	protobind.OK(c, &sacv1.GroupMemberListResponse{Members: convert.GroupMembersToProto(members)})
}

// ListMembersAdmin lists all members of a group (admin-only, no membership check).
func (h *Handler) ListMembersAdmin(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	ctx := context.Background()

	var members []models.GroupMember
	err := h.db.NewSelect().Model(&members).
		Where("gm.group_id = ?", groupID).
		Relation("User", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "username", "display_name")
		}).
		OrderExpr("gm.created_at ASC").
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list members", err)
		return
	}

	protobind.OK(c, &sacv1.GroupMemberListResponse{Members: convert.GroupMembersToProto(members)})
}

// AddMember adds a user to a group (admin-only endpoint).
func (h *Handler) AddMember(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	req := &sacv1.AddMemberRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.UserId == 0 {
		response.BadRequest(c, "user_id is required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	ctx := context.Background()

	// Verify target user exists
	var user models.User
	err := h.db.NewSelect().Model(&user).Where("id = ?", req.UserId).Scan(ctx)
	if err != nil {
		response.NotFound(c, "User not found")
		return
	}

	// Check if already a member
	if h.isMember(ctx, groupID, req.UserId) {
		response.Conflict(c, "User is already a member of this group")
		return
	}

	member := &models.GroupMember{
		GroupID:   groupID,
		UserID:    req.UserId,
		Role:      req.Role,
		CreatedAt: time.Now(),
	}

	_, err = h.db.NewInsert().Model(member).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to add member", err)
		return
	}

	protobind.Created(c, convert.GroupMemberToProto(member))
}

// RemoveMember removes a user from a group (admin-only endpoint).
func (h *Handler) RemoveMember(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	targetUserID, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	ctx := context.Background()

	// Don't allow removing the owner
	var group models.Group
	if err := h.db.NewSelect().Model(&group).Where("id = ?", groupID).Scan(ctx); err == nil {
		if group.OwnerID == targetUserID {
			response.BadRequest(c, "Cannot remove the group owner")
			return
		}
	}

	_, err = h.db.NewDelete().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, targetUserID).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to remove member", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Member removed"})
}

// UpdateMemberRole updates a member's role (admin-only endpoint).
func (h *Handler) UpdateMemberRole(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	targetUserID, err := strconv.ParseInt(c.Param("userId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	req := &sacv1.UpdateMemberRoleRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Role == "" {
		response.BadRequest(c, "role is required")
		return
	}

	ctx := context.Background()

	_, err = h.db.NewUpdate().Model((*models.GroupMember)(nil)).
		Set("role = ?", req.Role).
		Where("group_id = ? AND user_id = ?", groupID, targetUserID).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update member role", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Member role updated"})
}

// --- Helpers ---

func parseGroupID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return 0, false
	}
	return id, true
}

func (h *Handler) isMember(ctx context.Context, groupID, userID int64) bool {
	exists, _ := h.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Exists(ctx)
	return exists
}

func (h *Handler) isGroupAdmin(ctx context.Context, groupID, userID int64) bool {
	exists, _ := h.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ? AND role = 'admin'", groupID, userID).
		Exists(ctx)
	return exists
}

// GetTemplate returns the CLAUDE.md template for a group (members can read).
func (h *Handler) GetTemplate(c *gin.Context) {
	userID := c.GetInt64("userID")
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	ctx := context.Background()

	if !h.isMember(ctx, groupID, userID) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	var group models.Group
	err := h.db.NewSelect().Model(&group).
		Column("claude_md_template").
		Where("id = ?", groupID).
		Scan(ctx)
	if err != nil {
		response.NotFound(c, "Group not found")
		return
	}

	protobind.OK(c, &sacv1.GroupTemplateResponse{
		ClaudeMdTemplate: group.ClaudeMDTemplate,
	})
}

// UpdateTemplate updates the CLAUDE.md template (group admin or system admin).
func (h *Handler) UpdateTemplate(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}

	req := &sacv1.UpdateTemplateRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	ctx := context.Background()

	// Allow system admins or group admins
	role, _ := c.Get("role")
	userID := c.GetInt64("userID")
	if role != "admin" && !h.isGroupAdmin(ctx, groupID, userID) {
		response.Forbidden(c, "Only group admins or system admins can update the template")
		return
	}

	_, err := h.db.NewUpdate().Model((*models.Group)(nil)).
		Set("claude_md_template = ?", req.ClaudeMdTemplate).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", groupID).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update template", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Template updated"})
}

func groupIDs(groups []models.Group) []int64 {
	ids := make([]int64, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	return ids
}

func (h *Handler) getMemberCounts(ctx context.Context, ids []int64) map[int64]int {
	type countResult struct {
		GroupID int64 `bun:"group_id"`
		Count   int   `bun:"count"`
	}
	var counts []countResult
	_ = h.db.NewSelect().
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
