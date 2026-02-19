package workspace

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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

	// Notify subscribers via Redis
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

	// Notify subscribers via Redis
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
	c.Header("Content-Type", contentTypeByFilename(fileName))
	io.Copy(c.Writer, body)
}

// DeleteOutputFile deletes a file from the output workspace (user-facing, JWT required).
func (h *Handler) DeleteOutputFile(c *gin.Context) {
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

	ctx := context.Background()

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
		_, _ = h.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'output' AND oss_key LIKE ?", userIDInt, agentID, ossKey+"%").
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

	// Also clean up any shared links for this file
	_, _ = h.db.NewDelete().Model((*models.SharedLink)(nil)).
		Where("user_id = ? AND agent_id = ? AND file_path = ?", userIDInt, agentID, filePath).
		Exec(ctx)

	// Notify subscribers via Redis
	if h.hub != nil {
		h.hub.Publish(ctx, userIDInt, agentID, OutputEvent{
			Action: "delete",
			Path:   filePath,
			Name:   path.Base(filePath),
		})
	}

	response.Success(c, "File deleted")
}

// ---- Shared Links ----

// CreateShare creates a short link for an output workspace file.
func (h *Handler) CreateShare(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	var req struct {
		AgentID int64  `json:"agent_id" binding:"required"`
		Path    string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "agent_id and path are required", err)
		return
	}

	filePath := sanitizePath(req.Path)
	ossKey := outputOSSKeyPrefix(userIDInt, req.AgentID) + filePath
	fileName := path.Base(filePath)

	ctx := context.Background()

	// Check if already shared
	var existing models.SharedLink
	err := h.db.NewSelect().Model(&existing).
		Where("user_id = ? AND agent_id = ? AND file_path = ?", userIDInt, req.AgentID, filePath).
		Scan(ctx)
	if err == nil {
		// Already shared, return existing link
		response.OK(c, gin.H{
			"short_code": existing.ShortCode,
			"url":        "/s/" + existing.ShortCode,
		})
		return
	}

	// Verify file exists in OSS
	backend := h.provider.GetClient(ctx)
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}
	body, err := backend.Download(ctx, ossKey)
	if err != nil {
		response.NotFound(c, "File not found in output workspace", err)
		return
	}
	body.Close()

	// Generate short code
	shortCode := uuid.New().String()[:8]

	link := &models.SharedLink{
		ShortCode: shortCode,
		UserID:    userIDInt,
		AgentID:   req.AgentID,
		FilePath:  filePath,
		OSSKey:    ossKey,
		FileName:  fileName,
		CreatedAt: time.Now(),
	}

	_, err = h.db.NewInsert().Model(link).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create share link", err)
		return
	}

	response.Created(c, gin.H{
		"short_code": shortCode,
		"url":        "/s/" + shortCode,
	})
}

// DeleteShare removes a shared link.
func (h *Handler) DeleteShare(c *gin.Context) {
	userID, _ := c.Get("userID")
	userIDInt := userID.(int64)

	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "code parameter required")
		return
	}

	ctx := context.Background()
	result, err := h.db.NewDelete().Model((*models.SharedLink)(nil)).
		Where("short_code = ? AND user_id = ?", code, userIDInt).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to delete share link", err)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		response.NotFound(c, "Share link not found")
		return
	}

	response.Success(c, "Share link deleted")
}

// GetSharedFileMeta returns metadata for a shared file (public, no auth).
func (h *Handler) GetSharedFileMeta(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "code parameter required")
		return
	}

	ctx := context.Background()
	var link models.SharedLink
	err := h.db.NewSelect().Model(&link).Where("short_code = ?", code).Scan(ctx)
	if err != nil {
		response.NotFound(c, "Link not found or expired")
		return
	}

	contentType := contentTypeByFilename(link.FileName)

	// Try to get file size from workspace_files
	var sizeBytes int64
	_ = h.db.NewSelect().
		TableExpr("workspace_files").
		Column("size_bytes").
		Where("oss_key = ?", link.OSSKey).
		Scan(ctx, &sizeBytes)

	response.OK(c, gin.H{
		"file_name":    link.FileName,
		"content_type": contentType,
		"size_bytes":   sizeBytes,
	})
}

// DownloadSharedFile streams a shared file (public, no auth).
func (h *Handler) DownloadSharedFile(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BadRequest(c, "code parameter required")
		return
	}

	ctx := context.Background()
	var link models.SharedLink
	err := h.db.NewSelect().Model(&link).Where("short_code = ?", code).Scan(ctx)
	if err != nil {
		response.NotFound(c, "Link not found or expired")
		return
	}

	backend := h.provider.GetClient(ctx)
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}

	body, err := backend.Download(ctx, link.OSSKey)
	if err != nil {
		response.NotFound(c, "File no longer available", err)
		return
	}
	defer body.Close()

	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, link.FileName))
	c.Header("Content-Type", contentTypeByFilename(link.FileName))
	io.Copy(c.Writer, body)
}

// wsUpgrader upgrades HTTP connections to WebSocket.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WatchOutput is a WebSocket endpoint that pushes output workspace file events to the client.
// JWT is read from the "token" query parameter (WebSocket upgrade doesn't support Authorization header).
func (h *Handler) WatchOutput(c *gin.Context) {
	if h.hub == nil {
		response.ServiceUnavailable(c, "Output watch not available (Redis not configured)")
		return
	}

	// Manual JWT validation from query param
	tokenStr := c.Query("token")
	if tokenStr == "" {
		response.Unauthorized(c, "token query parameter required")
		return
	}
	claims, err := h.jwt.ValidateToken(tokenStr)
	if err != nil {
		response.Unauthorized(c, "Invalid or expired token")
		return
	}
	userID := claims.UserID

	agentIDStr := c.Query("agent_id")
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil || agentID <= 0 {
		response.BadRequest(c, "invalid agent_id")
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WatchOutput: websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	const (
		pingInterval = 30 * time.Second
		pongTimeout  = 60 * time.Second
	)

	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	// Drain reads (required by gorilla/websocket to process pong frames)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()

	ch, unsub := h.hub.Subscribe(userID, agentID)
	defer unsub()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-ch:
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("WatchOutput: marshal error: %v", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
