package workspace

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
)

// ---- Shared Workspace ----
// The shared workspace is a read-only space visible to all users.
// Files are published here from private workspaces via the Publish endpoint.

const sharedOSSPrefix = "shared/"

// ListSharedFiles lists files in the shared workspace.
func (h *Handler) ListSharedFiles(c *gin.Context) {
	oss := h.getOSS(c)
	reqPath := sanitizePath(c.DefaultQuery("path", "/"))
	prefix := sharedOSSPrefix + reqPath

	items, err := oss.List(prefix, "/", 1000)
	if err != nil {
		response.InternalError(c, "Failed to list shared files", err)
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
		relPath := strings.TrimPrefix(item.Key, sharedOSSPrefix)
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

// DownloadSharedFile downloads a file from the shared workspace.
func (h *Handler) DownloadSharedFile(c *gin.Context) {
	oss := h.getOSS(c)
	filePath := c.Query("path")
	if filePath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}
	filePath = sanitizePath(filePath)
	ossKey := sharedOSSPrefix + filePath

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

// PublishToShared copies a file from private workspace to the shared workspace (admin only).
func (h *Handler) PublishToShared(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "admin" {
		response.Forbidden(c, "Admin access required to publish to shared workspace")
		return
	}

	oss := h.getOSS(c)
	userIDInt := c.GetInt64("userID")

	var req struct {
		AgentID  int64  `json:"agent_id" binding:"required"`
		Path     string `json:"path" binding:"required"`
		DestPath string `json:"dest_path"` // optional: override destination in shared/
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "agent_id and path are required", err)
		return
	}

	srcPath := sanitizePath(req.Path)
	srcOSSKey := ossKeyPrefix(userIDInt, req.AgentID) + srcPath

	destPath := srcPath
	if req.DestPath != "" {
		destPath = sanitizePath(req.DestPath)
	}
	destOSSKey := sharedOSSPrefix + destPath

	ctx := context.Background()

	// Copy from private to shared
	if err := oss.Copy(srcOSSKey, destOSSKey); err != nil {
		response.InternalError(c, "Failed to publish file", err)
		return
	}

	// Get source file size
	srcSize, _ := oss.GetObjectSize(destOSSKey)

	// Upsert workspace_files record
	wf := &models.WorkspaceFile{
		UserID:        userIDInt,
		AgentID:       req.AgentID,
		WorkspaceType: "shared",
		OSSKey:        destOSSKey,
		FileName:      path.Base(destPath),
		FilePath:      destPath,
		ContentType:   "application/octet-stream",
		SizeBytes:     srcSize,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err := h.db.NewInsert().Model(wf).
		On("CONFLICT (oss_key) DO UPDATE").
		Set("size_bytes = EXCLUDED.size_bytes").
		Set("updated_at = EXCLUDED.updated_at").
		Set("user_id = EXCLUDED.user_id").
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to save shared file record", err)
		return
	}

	// Sync to all active pods
	go h.syncSvc.SyncSharedFileToPods(context.Background(), destOSSKey, destPath)

	response.Created(c, gin.H{
		"message": "File published to shared workspace",
		"path":    destPath,
	})
}

// DeleteSharedFile deletes a file from the shared workspace (admin only).
func (h *Handler) DeleteSharedFile(c *gin.Context) {
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
	ossKey := sharedOSSPrefix + filePath

	ctx := context.Background()

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("workspace_type = 'shared' AND oss_key LIKE ?", ossKey+"%").
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

	go h.syncSvc.DeleteSharedFileFromPods(context.Background(), filePath)

	response.Success(c, "File deleted")
}
