package workspace

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"path"
	"strconv"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

const maxUploadSize = 100 << 20 // 100MB

// Handler serves workspace HTTP endpoints.
type Handler struct {
	db       *bun.DB
	provider *storage.OSSProvider
	syncSvc  *SyncService
}

// NewHandler creates a new workspace handler.
func NewHandler(db *bun.DB, provider *storage.OSSProvider, syncSvc *SyncService) *Handler {
	return &Handler{db: db, provider: provider, syncSvc: syncSvc}
}

// RegisterRoutes registers workspace routes on a protected router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	ws := rg.Group("/workspace")
	{
		// OSS status (always available)
		ws.GET("/status", h.GetStatus)

		// Private workspace (per-agent)
		ws.POST("/upload", h.requireOSS(), h.Upload)
		ws.GET("/files", h.requireOSS(), h.ListFiles)
		ws.GET("/files/download", h.requireOSS(), h.DownloadFile)
		ws.DELETE("/files", h.requireOSS(), h.DeleteFile)
		ws.POST("/directories", h.requireOSS(), h.CreateDirectory)
		ws.GET("/quota", h.requireOSS(), h.GetQuota)

		// Public workspace
		ws.GET("/public/files", h.requireOSS(), h.ListPublicFiles)
		ws.GET("/public/files/download", h.requireOSS(), h.DownloadPublicFile)
		ws.POST("/public/upload", h.requireOSS(), h.UploadPublic)
		ws.POST("/public/directories", h.requireOSS(), h.CreatePublicDirectory)
		ws.DELETE("/public/files", h.requireOSS(), h.DeletePublicFile)

		// Group workspace
		ws.GET("/group/files", h.requireOSS(), h.ListGroupFiles)
		ws.GET("/group/files/download", h.requireOSS(), h.DownloadGroupFile)
		ws.POST("/group/upload", h.requireOSS(), h.UploadGroup)
		ws.POST("/group/directories", h.requireOSS(), h.CreateGroupDirectory)
		ws.DELETE("/group/files", h.requireOSS(), h.DeleteGroupFile)
		ws.GET("/group/quota", h.requireOSS(), h.GetGroupQuota)

		// Shared workspace (read-only browsing + publish)
		ws.GET("/shared/files", h.requireOSS(), h.ListSharedFiles)
		ws.GET("/shared/files/download", h.requireOSS(), h.DownloadSharedFile)
		ws.POST("/shared/publish", h.requireOSS(), h.PublishToShared)
		ws.DELETE("/shared/files", h.requireOSS(), h.DeleteSharedFile)
	}
}

// requireOSS is a middleware that checks if OSS is configured.
func (h *Handler) requireOSS() gin.HandlerFunc {
	return func(c *gin.Context) {
		oss := h.provider.GetClient(c.Request.Context())
		if oss == nil {
			response.ServiceUnavailable(c, "Workspace storage is not configured. Ask your admin to set OSS settings.")
			c.Abort()
			return
		}
		// Store client in context for the handler to use
		c.Set("ossClient", oss)
		c.Next()
	}
}

// getOSS retrieves the OSSClient stored by requireOSS middleware.
func (h *Handler) getOSS(c *gin.Context) *storage.OSSClient {
	v, _ := c.Get("ossClient")
	return v.(*storage.OSSClient)
}

// GetStatus returns whether OSS is configured.
func (h *Handler) GetStatus(c *gin.Context) {
	configured := h.provider.IsConfigured(c.Request.Context())
	response.OK(c, gin.H{"configured": configured})
}

// parseAgentID extracts and validates the agent_id parameter from query or form.
func parseAgentID(c *gin.Context) (int64, bool) {
	agentIDStr := c.Query("agent_id")
	if agentIDStr == "" {
		agentIDStr = c.PostForm("agent_id")
	}
	if agentIDStr == "" {
		response.BadRequest(c, "agent_id parameter required")
		return 0, false
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil || agentID <= 0 {
		response.BadRequest(c, "invalid agent_id")
		return 0, false
	}
	return agentID, true
}

// ossKeyPrefix returns the OSS key prefix for a user's agent workspace.
func ossKeyPrefix(userID, agentID int64) string {
	return fmt.Sprintf("users/%d/agents/%d/", userID, agentID)
}

// ---- Private Workspace (per-agent) ----

// Upload handles multipart file upload to private workspace.
func (h *Handler) Upload(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
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

	ctx := context.Background()

	// Check quota
	quota := h.getOrCreateQuota(ctx, userIDInt, agentID)
	if quota.UsedBytes+header.Size > quota.MaxBytes {
		response.BadRequest(c, fmt.Sprintf("Quota exceeded: used %d + file %d > max %d", quota.UsedBytes, header.Size, quota.MaxBytes))
		return
	}
	if quota.FileCount >= quota.MaxFileCount {
		response.BadRequest(c, fmt.Sprintf("File count limit exceeded: %d/%d", quota.FileCount, quota.MaxFileCount))
		return
	}

	ossKey := ossKeyPrefix(userIDInt, agentID) + filePath + header.Filename

	checksum, err := uploadToOSS(oss, ossKey, file, header)
	if err != nil {
		response.InternalError(c, "Failed to upload file", err)
		return
	}

	// Upsert workspace_files record
	wf := &models.WorkspaceFile{
		UserID:        userIDInt,
		AgentID:       agentID,
		WorkspaceType: "private",
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

	// Update quota
	h.recalcQuota(ctx, userIDInt, agentID)

	// Sync to active pods in background
	go h.syncSvc.SyncFileToPods(context.Background(), userIDInt, agentID, ossKey, filePath+header.Filename)

	response.Created(c, wf)
}

// ListFiles lists files in the user's agent-specific private workspace.
func (h *Handler) ListFiles(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	reqPath := sanitizePath(c.DefaultQuery("path", "/"))
	prefix := ossKeyPrefix(userIDInt, agentID) + reqPath

	items, err := oss.List(prefix, "/", 1000)
	if err != nil {
		response.InternalError(c, "Failed to list files", err)
		return
	}

	// Map to response format
	type FileItem struct {
		Name         string    `json:"name"`
		Path         string    `json:"path"`
		Size         int64     `json:"size"`
		IsDirectory  bool      `json:"is_directory"`
		LastModified time.Time `json:"last_modified,omitzero"`
	}

	basePrefix := ossKeyPrefix(userIDInt, agentID)
	var files []FileItem
	for _, item := range items {
		// Strip the user/agent prefix to get the relative path
		relPath := strings.TrimPrefix(item.Key, basePrefix)
		name := path.Base(relPath)
		if item.IsDirectory {
			relPath = strings.TrimPrefix(item.Key, basePrefix)
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

// DownloadFile downloads a private workspace file.
func (h *Handler) DownloadFile(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)

	ossKey := ossKeyPrefix(userIDInt, agentID) + filePath

	body, err := oss.Download(ossKey)
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

// DeleteFile deletes a private workspace file.
func (h *Handler) DeleteFile(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := ossKeyPrefix(userIDInt, agentID) + filePath

	ctx := context.Background()

	// Check if it's a directory prefix
	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		// Delete DB records with prefix
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'private' AND oss_key LIKE ?", userIDInt, agentID, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ossKey); err != nil {
			response.InternalError(c, "Failed to delete file", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	h.recalcQuota(ctx, userIDInt, agentID)

	// Delete from active pods in background
	go h.syncSvc.DeleteFileFromPods(context.Background(), userIDInt, agentID, filePath)

	response.Success(c, "File deleted")
}

// CreateDirectory creates a directory marker in private workspace.
func (h *Handler) CreateDirectory(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	var req struct {
		Path    string `json:"path" binding:"required"`
		AgentID int64  `json:"agent_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "path and agent_id are required", err)
		return
	}

	if req.AgentID <= 0 {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := ossKeyPrefix(userIDInt, req.AgentID) + dirPath

	// Create an empty object as directory marker
	if err := oss.Upload(ossKey, strings.NewReader(""), "application/x-directory"); err != nil {
		response.InternalError(c, "Failed to create directory", err)
		return
	}

	response.Created(c, gin.H{"path": dirPath})
}

// GetQuota returns the user's workspace quota for a specific agent.
func (h *Handler) GetQuota(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	ctx := context.Background()
	quota := h.getOrCreateQuota(ctx, userIDInt, agentID)
	response.OK(c, quota)
}

// ---- Public Workspace ----

// ListPublicFiles lists files in the public workspace.
func (h *Handler) ListPublicFiles(c *gin.Context) {
	oss := h.getOSS(c)
	reqPath := sanitizePath(c.DefaultQuery("path", "/"))
	prefix := "public/" + reqPath

	items, err := oss.List(prefix, "/", 1000)
	if err != nil {
		response.InternalError(c, "Failed to list public files", err)
		return
	}

	type FileItem struct {
		Name         string    `json:"name"`
		Path         string    `json:"path"`
		Size         int64     `json:"size"`
		IsDirectory  bool      `json:"is_directory"`
		LastModified time.Time `json:"last_modified,omitzero"`
	}

	var files []FileItem
	for _, item := range items {
		relPath := strings.TrimPrefix(item.Key, "public/")
		name := path.Base(relPath)
		if item.IsDirectory {
			relPath = strings.TrimPrefix(item.Key, "public/")
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

// DownloadPublicFile downloads a file from public workspace.
func (h *Handler) DownloadPublicFile(c *gin.Context) {
	oss := h.getOSS(c)
	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := "public/" + filePath

	body, err := oss.Download(ossKey)
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

// CreatePublicDirectory creates a directory marker in public workspace (admin only).
func (h *Handler) CreatePublicDirectory(c *gin.Context) {
	oss := h.getOSS(c)
	role, _ := c.Get("role")
	if role != "admin" {
		response.Forbidden(c, "Admin access required")
		return
	}

	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "path is required", err)
		return
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := "public/" + dirPath

	if err := oss.Upload(ossKey, strings.NewReader(""), "application/x-directory"); err != nil {
		response.InternalError(c, "Failed to create directory", err)
		return
	}

	response.Created(c, gin.H{"path": dirPath})
}

// UploadPublic handles file upload to public workspace (admin only).
func (h *Handler) UploadPublic(c *gin.Context) {
	oss := h.getOSS(c)
	role, _ := c.Get("role")
	if role != "admin" {
		response.Forbidden(c, "Admin access required")
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

	ossKey := "public/" + filePath + header.Filename

	checksum, err := uploadToOSS(oss, ossKey, file, header)
	if err != nil {
		response.InternalError(c, "Failed to upload file", err)
		return
	}

	ctx := context.Background()
	wf := &models.WorkspaceFile{
		UserID:        0, // public
		AgentID:       0, // public
		WorkspaceType: "public",
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

	// Sync public file to all active pods in background
	go h.syncSvc.SyncPublicFileToPods(context.Background(), ossKey, filePath+header.Filename)

	response.Created(c, wf)
}

// DeletePublicFile deletes a file from public workspace (admin only).
func (h *Handler) DeletePublicFile(c *gin.Context) {
	oss := h.getOSS(c)
	role, _ := c.Get("role")
	if role != "admin" {
		response.Forbidden(c, "Admin access required")
		return
	}

	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := "public/" + filePath

	ctx := context.Background()

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("workspace_type = 'public' AND oss_key LIKE ?", ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ossKey); err != nil {
			response.InternalError(c, "Failed to delete file", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	// Delete public file from all active pods in background
	go h.syncSvc.DeletePublicFileFromPods(context.Background(), filePath)

	response.Success(c, "File deleted")
}

// ---- Helpers ----

func uploadToOSS(oss *storage.OSSClient, ossKey string, file multipart.File, header *multipart.FileHeader) (string, error) {
	// Compute MD5 checksum
	hasher := md5.New()
	tee := io.TeeReader(file, hasher)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if err := oss.Upload(ossKey, tee, contentType); err != nil {
		return "", err
	}

	checksum := fmt.Sprintf("%x", hasher.Sum(nil))
	return checksum, nil
}

func (h *Handler) getOrCreateQuota(ctx context.Context, userID, agentID int64) *models.WorkspaceQuota {
	var quota models.WorkspaceQuota
	err := h.db.NewSelect().Model(&quota).Where("user_id = ? AND agent_id = ?", userID, agentID).Scan(ctx)
	if err != nil {
		// Create default quota
		quota = models.WorkspaceQuota{
			UserID:       userID,
			AgentID:      agentID,
			UsedBytes:    0,
			MaxBytes:     1 << 30, // 1GB
			FileCount:    0,
			MaxFileCount: 1000,
			UpdatedAt:    time.Now(),
		}
		_, _ = h.db.NewInsert().Model(&quota).Exec(ctx)
	}
	return &quota
}

func (h *Handler) recalcQuota(ctx context.Context, userID, agentID int64) {
	var result struct {
		TotalSize int64 `bun:"total_size"`
		FileCount int   `bun:"file_count"`
	}

	err := h.db.NewSelect().
		TableExpr("workspace_files").
		ColumnExpr("COALESCE(SUM(size_bytes), 0) AS total_size").
		ColumnExpr("COUNT(*) AS file_count").
		Where("user_id = ? AND agent_id = ? AND workspace_type = 'private' AND is_directory = FALSE", userID, agentID).
		Scan(ctx, &result)

	if err != nil {
		log.Printf("Warning: failed to recalc quota for user %d agent %d: %v", userID, agentID, err)
		return
	}

	_, _ = h.db.NewUpdate().Model((*models.WorkspaceQuota)(nil)).
		Set("used_bytes = ?", result.TotalSize).
		Set("file_count = ?", result.FileCount).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ? AND agent_id = ?", userID, agentID).
		Exec(ctx)
}

// sanitizePath cleans a file path, preventing directory traversal.
func sanitizePath(p string) string {
	// Remove any leading/trailing whitespace
	p = strings.TrimSpace(p)
	// Remove leading slashes
	p = strings.TrimLeft(p, "/")
	// Remove dangerous path components
	p = strings.ReplaceAll(p, "..", "")
	p = strings.ReplaceAll(p, "//", "/")
	return p
}
