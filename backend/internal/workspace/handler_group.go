package workspace

import (
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
)

// ---- Group Workspace ----

// parseGroupIDParam extracts group_id from query or form.
func parseGroupIDParam(c *gin.Context) (int64, bool) {
	s := c.Query("group_id")
	if s == "" {
		s = c.PostForm("group_id")
	}
	if s == "" {
		response.BadRequest(c, "group_id parameter required")
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid group_id")
		return 0, false
	}
	return id, true
}

// groupOSSKeyPrefix returns the OSS key prefix for a group workspace.
func groupOSSKeyPrefix(groupID int64) string {
	return fmt.Sprintf("groups/%d/", groupID)
}

// isGroupMember checks if the current user is a member of the group.
func (h *Handler) isGroupMember(ctx context.Context, groupID, userID int64) bool {
	exists, _ := h.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Exists(ctx)
	return exists
}

// ListGroupFiles lists files in a group workspace.
func (h *Handler) ListGroupFiles(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	groupID, ok := parseGroupIDParam(c)
	if !ok {
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, groupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	reqPath := sanitizePath(c.DefaultQuery("path", "/"))
	prefix := groupOSSKeyPrefix(groupID) + reqPath

	items, err := oss.List(c.Request.Context(), prefix, "/", 1000)
	if err != nil {
		response.InternalError(c, "Failed to list group files", err)
		return
	}

	type FileItem struct {
		Name         string    `json:"name"`
		Path         string    `json:"path"`
		Size         int64     `json:"size"`
		IsDirectory  bool      `json:"is_directory"`
		LastModified time.Time `json:"last_modified,omitzero"`
	}

	basePrefix := groupOSSKeyPrefix(groupID)
	var files []FileItem
	for _, item := range items {
		relPath := strings.TrimPrefix(item.Key, basePrefix)
		name := path.Base(relPath)
		if item.IsDirectory {
			name = path.Base(strings.TrimSuffix(relPath, "/"))
			if name == "." || name == "" {
				continue
			}
		}
		files = append(files, FileItem{
			Name:         name,
			Path:         relPath,
			Size:         item.Size,
			IsDirectory:  item.IsDirectory,
			LastModified: item.LastModified,
		})
	}

	response.OK(c, gin.H{
		"path":  reqPath,
		"files": files,
	})
}

// DownloadGroupFile downloads a file from a group workspace.
func (h *Handler) DownloadGroupFile(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	groupID, ok := parseGroupIDParam(c)
	if !ok {
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, groupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := groupOSSKeyPrefix(groupID) + filePath

	body, err := oss.Download(c.Request.Context(), ossKey)
	if err != nil {
		response.NotFound(c, "File not found", err)
		return
	}
	defer body.Close()

	fileName := path.Base(filePath)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Header("Content-Type", "application/octet-stream")
	io.Copy(c.Writer, body)
}

// UploadGroup handles file upload to a group workspace.
func (h *Handler) UploadGroup(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	groupID, ok := parseGroupIDParam(c)
	if !ok {
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, groupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	filePath := c.DefaultPostForm("path", "/")
	filePath = sanitizePath(filePath)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "No file provided", err)
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		response.BadRequest(c, fmt.Sprintf("File too large: %d bytes (max %d)", header.Size, maxUploadSize))
		return
	}

	// Check group quota
	quota := h.getOrCreateGroupQuota(ctx, groupID)
	if quota.UsedBytes+header.Size > quota.MaxBytes {
		response.BadRequest(c, fmt.Sprintf("Group quota exceeded: used %d + file %d > max %d", quota.UsedBytes, header.Size, quota.MaxBytes))
		return
	}
	if quota.FileCount >= quota.MaxFileCount {
		response.BadRequest(c, fmt.Sprintf("Group file count limit exceeded: %d/%d", quota.FileCount, quota.MaxFileCount))
		return
	}

	ossKey := groupOSSKeyPrefix(groupID) + filePath + header.Filename

	checksum, err := uploadToStorage(ctx, oss, ossKey, file, header)
	if err != nil {
		response.InternalError(c, "Failed to upload file", err)
		return
	}

	wf := &models.WorkspaceFile{
		UserID:        userIDInt,
		AgentID:       0,
		GroupID:       &groupID,
		WorkspaceType: "group",
		OSSKey:        ossKey,
		FileName:      header.Filename,
		FilePath:      filePath + header.Filename,
		ContentType:   header.Header.Get("Content-Type"),
		SizeBytes:     header.Size,
		Checksum:      checksum,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err = h.db.NewInsert().Model(wf).
		On("CONFLICT (oss_key) DO UPDATE").
		Set("size_bytes = EXCLUDED.size_bytes").
		Set("checksum = EXCLUDED.checksum").
		Set("content_type = EXCLUDED.content_type").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to save file record", err)
		return
	}

	h.recalcGroupQuota(ctx, groupID)

	// Sync to active pods of group members in background
	go h.syncSvc.SyncGroupFileToPods(context.Background(), groupID, ossKey, filePath+header.Filename)

	response.Created(c, wf)
}

// CreateGroupDirectory creates a directory in group workspace.
func (h *Handler) CreateGroupDirectory(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	var req struct {
		Path    string `json:"path" binding:"required"`
		GroupID int64  `json:"group_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "path and group_id are required", err)
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, req.GroupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := groupOSSKeyPrefix(req.GroupID) + dirPath

	if err := oss.Upload(c.Request.Context(), ossKey, strings.NewReader(""), 0, "application/x-directory"); err != nil {
		response.InternalError(c, "Failed to create directory", err)
		return
	}

	response.Created(c, gin.H{"path": dirPath})
}

// DeleteGroupFile deletes a file from a group workspace.
func (h *Handler) DeleteGroupFile(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	groupID, ok := parseGroupIDParam(c)
	if !ok {
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, groupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := groupOSSKeyPrefix(groupID) + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("group_id = ? AND workspace_type = 'group' AND oss_key LIKE ?", groupID, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			response.InternalError(c, "Failed to delete file", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	h.recalcGroupQuota(ctx, groupID)

	go h.syncSvc.DeleteGroupFileFromPods(context.Background(), groupID, filePath)

	response.Success(c, "File deleted")
}

// GetGroupQuota returns a group's workspace quota.
func (h *Handler) GetGroupQuota(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	groupID, ok := parseGroupIDParam(c)
	if !ok {
		return
	}

	ctx := context.Background()
	if !h.isGroupMember(ctx, groupID, userIDInt) {
		response.Forbidden(c, "Not a member of this group")
		return
	}

	quota := h.getOrCreateGroupQuota(ctx, groupID)
	response.OK(c, quota)
}

// --- Group quota helpers ---

func (h *Handler) getOrCreateGroupQuota(ctx context.Context, groupID int64) *models.GroupWorkspaceQuota {
	var quota models.GroupWorkspaceQuota
	err := h.db.NewSelect().Model(&quota).Where("group_id = ?", groupID).Scan(ctx)
	if err != nil {
		quota = models.GroupWorkspaceQuota{
			GroupID:      groupID,
			UsedBytes:    0,
			MaxBytes:     1 << 30,
			FileCount:    0,
			MaxFileCount: 1000,
			UpdatedAt:    time.Now(),
		}
		_, _ = h.db.NewInsert().Model(&quota).Exec(ctx)
	}
	return &quota
}

func (h *Handler) recalcGroupQuota(ctx context.Context, groupID int64) {
	var result struct {
		TotalSize int64 `bun:"total_size"`
		FileCount int   `bun:"file_count"`
	}

	err := h.db.NewSelect().
		TableExpr("workspace_files").
		ColumnExpr("COALESCE(SUM(size_bytes), 0) AS total_size").
		ColumnExpr("COUNT(*) AS file_count").
		Where("group_id = ? AND workspace_type = 'group' AND is_directory = FALSE", groupID).
		Scan(ctx, &result)

	if err != nil {
		log.Printf("Warning: failed to recalc group quota for group %d: %v", groupID, err)
		return
	}

	_, _ = h.db.NewUpdate().Model((*models.GroupWorkspaceQuota)(nil)).
		Set("used_bytes = ?", result.TotalSize).
		Set("file_count = ?", result.FileCount).
		Set("updated_at = ?", time.Now()).
		Where("group_id = ?", groupID).
		Exec(ctx)
}
