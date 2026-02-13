package workspace

import (
	"context"
	"crypto/md5"
	"encoding/json"
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

const outputOSSPrefix = "output"

// outputOSSKeyPrefix returns the OSS key prefix for a user's agent output workspace.
func outputOSSKeyPrefix(userID, agentID int64) string {
	return fmt.Sprintf("users/%d/agents/%d/%s/", userID, agentID, outputOSSPrefix)
}

// RegisterInternalRoutes registers internal routes (no JWT, sidecar calls).
func (h *Handler) RegisterInternalRoutes(rg *gin.RouterGroup) {
	out := rg.Group("/output")
	{
		out.POST("/upload", h.requireOSS(), h.InternalOutputUpload)
		out.POST("/delete", h.requireOSS(), h.InternalOutputDelete)
	}
}

// InternalOutputUpload handles multipart file upload from sidecar to output workspace.
func (h *Handler) InternalOutputUpload(c *gin.Context) {
	oss := h.getOSS(c)

	userIDStr := c.PostForm("user_id")
	agentIDStr := c.PostForm("agent_id")
	filePath := c.PostForm("path")

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user_id")
		return
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil || agentID <= 0 {
		response.BadRequest(c, "invalid agent_id")
		return
	}
	if filePath == "" {
		response.BadRequest(c, "path is required")
		return
	}
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
	ossKey := outputOSSKeyPrefix(userID, agentID) + filePath

	// Compute MD5 checksum while uploading
	hasher := md5.New()
	tee := io.TeeReader(file, hasher)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if err := oss.Upload(ctx, ossKey, tee, header.Size, contentType); err != nil {
		response.InternalError(c, "Failed to upload file", err)
		return
	}

	checksum := fmt.Sprintf("%x", hasher.Sum(nil))

	// Upsert workspace_files record
	wf := &models.WorkspaceFile{
		UserID:        userID,
		AgentID:       agentID,
		WorkspaceType: "output",
		OSSKey:        ossKey,
		FileName:      path.Base(filePath),
		FilePath:      filePath,
		ContentType:   contentType,
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

	// Notify SSE subscribers via Redis
	if h.hub != nil {
		h.hub.Publish(ctx, userID, agentID, OutputEvent{
			Action: "upload",
			Path:   filePath,
			Name:   path.Base(filePath),
			Size:   header.Size,
		})
	}

	response.Created(c, wf)
}

// InternalOutputDelete handles file deletion from sidecar.
func (h *Handler) InternalOutputDelete(c *gin.Context) {
	oss := h.getOSS(c)

	var req struct {
		UserID  int64  `json:"user_id" binding:"required"`
		AgentID int64  `json:"agent_id" binding:"required"`
		Path    string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "user_id, agent_id, and path are required", err)
		return
	}

	filePath := sanitizePath(req.Path)
	ossKey := outputOSSKeyPrefix(req.UserID, req.AgentID) + filePath

	ctx := context.Background()

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'output' AND oss_key LIKE ?", req.UserID, req.AgentID, ossKey+"%").
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

	// Notify SSE subscribers via Redis
	if h.hub != nil {
		h.hub.Publish(ctx, req.UserID, req.AgentID, OutputEvent{
			Action: "delete",
			Path:   filePath,
			Name:   path.Base(filePath),
		})
	}

	response.Success(c, "File deleted")
}

// ListOutputFiles lists files in the user's agent output workspace.
func (h *Handler) ListOutputFiles(c *gin.Context) {
	oss := h.getOSS(c)
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	reqPath := sanitizePath(c.DefaultQuery("path", "/"))
	prefix := outputOSSKeyPrefix(userIDInt, agentID) + reqPath

	items, err := oss.List(c.Request.Context(), prefix, "/", 1000)
	if err != nil {
		response.InternalError(c, "Failed to list output files", err)
		return
	}

	type FileItem struct {
		Name         string    `json:"name"`
		Path         string    `json:"path"`
		Size         int64     `json:"size"`
		IsDirectory  bool      `json:"is_directory"`
		LastModified time.Time `json:"last_modified,omitzero"`
	}

	basePrefix := outputOSSKeyPrefix(userIDInt, agentID)
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

// DownloadOutputFile downloads a file from the output workspace.
func (h *Handler) DownloadOutputFile(c *gin.Context) {
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

	ossKey := outputOSSKeyPrefix(userIDInt, agentID) + filePath

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

// WatchOutput is an SSE endpoint that pushes output workspace file events to the client.
func (h *Handler) WatchOutput(c *gin.Context) {
	if h.hub == nil {
		response.ServiceUnavailable(c, "Output watch not available (Redis not configured)")
		return
	}

	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	agentID, ok := parseAgentID(c)
	if !ok {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(userIDInt, agentID)
	defer unsub()

	flusher := c.Writer
	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("WatchOutput: marshal error: %v", err)
				continue
			}
			fmt.Fprintf(flusher, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
