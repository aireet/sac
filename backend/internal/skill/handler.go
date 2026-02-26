package skill

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db          *bun.DB
	syncService *SyncService
}

func NewHandler(db *bun.DB, containerManager *container.Manager, storageProvider *storage.StorageProvider) *Handler {
	return &Handler{
		db:          db,
		syncService: NewSyncService(db, containerManager, storageProvider),
	}
}

// GetSyncService returns the handler's SyncService for use by other handlers.
func (h *Handler) GetSyncService() *SyncService {
	return h.syncService
}

// RegisterFileRoutes registers skill file management routes (multipart upload, not suitable for gRPC-gateway).
func (h *Handler) RegisterFileRoutes(router *gin.RouterGroup) {
	router.POST("/skills/:id/files", h.UploadSkillFile)
	router.GET("/skills/:id/files", h.ListSkillFiles)
	router.DELETE("/skills/:id/files", h.DeleteSkillFile)
	router.GET("/skills/:id/files/download", h.DownloadSkillFile)
	router.PUT("/skills/:id/files/content", h.SaveSkillFileContent)
	router.GET("/skills/:id/files/content", h.GetSkillFileContent)
}

// UploadSkillFile handles multipart file upload for a skill.
func (h *Handler) UploadSkillFile(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "No file provided", err)
		return
	}
	defer file.Close()

	filepath := c.PostForm("filepath")
	if filepath == "" {
		filepath = header.Filename
	}

	if h.syncService.storage == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}
	backend := h.syncService.storage.GetClient(c.Request.Context())
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}

	s3Key := fmt.Sprintf("skills/%d/%s", skillID, filepath)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Read file and compute MD5 checksum
	ctx := context.Background()
	data, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "Failed to read file", err)
		return
	}
	hash := md5.Sum(data)
	checksum := hex.EncodeToString(hash[:])

	if err := backend.Upload(ctx, s3Key, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
		response.InternalError(c, "Failed to upload file", err)
		return
	}

	sf := &models.SkillFile{
		SkillID:     skillID,
		Filepath:    filepath,
		S3Key:       s3Key,
		Checksum:    checksum,
		Size:        int64(len(data)),
		ContentType: contentType,
		CreatedAt:   time.Now(),
	}

	_, err = h.db.NewInsert().Model(sf).
		On("CONFLICT (skill_id, filepath) DO UPDATE").
		Set("s3_key = EXCLUDED.s3_key").
		Set("checksum = EXCLUDED.checksum").
		Set("size = EXCLUDED.size").
		Set("content_type = EXCLUDED.content_type").
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to save file record", err)
		return
	}

	// Recompute content_checksum and bump version
	if err := h.syncService.RebuildSkillBundle(ctx, skillID); err != nil {
		response.InternalError(c, "Failed to recompute checksum", err)
		return
	}

	protobind.Created(c, convert.SkillFileToProto(sf))
}

// ListSkillFiles lists all files attached to a skill.
func (h *Handler) ListSkillFiles(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	ctx := context.Background()
	var files []models.SkillFile
	err = h.db.NewSelect().Model(&files).Where("skill_id = ?", skillID).Order("filepath ASC").Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to list files", err)
		return
	}

	protobind.OK(c, &sacv1.SkillFileListResponse{Files: convert.SkillFilesToProto(files)})
}

// DeleteSkillFile deletes a file (or directory) attached to a skill.
func (h *Handler) DeleteSkillFile(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	filepath := c.Query("path")
	if filepath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}

	ctx := context.Background()

	// Directory delete: path ends with "/"
	if strings.HasSuffix(filepath, "/") {
		prefix := fmt.Sprintf("skills/%d/%s", skillID, filepath)

		if h.syncService.storage != nil {
			if backend := h.syncService.storage.GetClient(ctx); backend != nil {
				_ = backend.DeletePrefix(ctx, prefix)
			}
		}

		_, err = h.db.NewDelete().Model((*models.SkillFile)(nil)).
			Where("skill_id = ? AND filepath LIKE ?", skillID, filepath+"%").
			Exec(ctx)
		if err != nil {
			response.InternalError(c, "Failed to delete directory", err)
			return
		}
	} else {
		var sf models.SkillFile
		err = h.db.NewSelect().Model(&sf).Where("skill_id = ? AND filepath = ?", skillID, filepath).Scan(ctx)
		if err != nil {
			response.NotFound(c, "File not found", err)
			return
		}

		if h.syncService.storage != nil {
			if backend := h.syncService.storage.GetClient(ctx); backend != nil {
				_ = backend.Delete(ctx, sf.S3Key)
			}
		}

		_, err = h.db.NewDelete().Model(&sf).Where("id = ?", sf.ID).Exec(ctx)
		if err != nil {
			response.InternalError(c, "Failed to delete file", err)
			return
		}
	}

	// Recompute content_checksum and bump version
	if err := h.syncService.RebuildSkillBundle(ctx, skillID); err != nil {
		response.InternalError(c, "Failed to recompute checksum", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "File deleted"})
}

// DownloadSkillFile downloads a file attached to a skill.
func (h *Handler) DownloadSkillFile(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	filepath := c.Query("path")
	if filepath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}

	ctx := context.Background()

	var sf models.SkillFile
	err = h.db.NewSelect().Model(&sf).Where("skill_id = ? AND filepath = ?", skillID, filepath).Scan(ctx)
	if err != nil {
		response.NotFound(c, "File not found", err)
		return
	}

	if h.syncService.storage == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}
	backend := h.syncService.storage.GetClient(ctx)
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}

	body, err := backend.Download(ctx, sf.S3Key)
	if err != nil {
		response.NotFound(c, "File not found in storage", err)
		return
	}
	defer body.Close()

	c.Header("Content-Type", sf.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, path.Base(filepath)))
	io.Copy(c.Writer, body)
}

// SaveSkillFileContent saves text content as a skill file.
func (h *Handler) SaveSkillFileContent(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	req := &sacv1.SaveSkillFileContentRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Filepath == "" {
		response.BadRequest(c, "filepath is required")
		return
	}

	if h.syncService.storage == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}
	backend := h.syncService.storage.GetClient(c.Request.Context())
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}

	ctx := context.Background()
	s3Key := fmt.Sprintf("skills/%d/%s", skillID, req.Filepath)
	contentType := "text/plain"

	// Compute MD5 checksum
	contentBytes := []byte(req.Content)
	hash := md5.Sum(contentBytes)
	checksum := hex.EncodeToString(hash[:])

	if err := backend.Upload(ctx, s3Key, strings.NewReader(req.Content), int64(len(req.Content)), contentType); err != nil {
		response.InternalError(c, "Failed to upload file content", err)
		return
	}

	sf := &models.SkillFile{
		SkillID:     skillID,
		Filepath:    req.Filepath,
		S3Key:       s3Key,
		Checksum:    checksum,
		Size:        int64(len(req.Content)),
		ContentType: contentType,
		CreatedAt:   time.Now(),
	}

	_, err = h.db.NewInsert().Model(sf).
		On("CONFLICT (skill_id, filepath) DO UPDATE").
		Set("s3_key = EXCLUDED.s3_key").
		Set("checksum = EXCLUDED.checksum").
		Set("size = EXCLUDED.size").
		Set("content_type = EXCLUDED.content_type").
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to save file record", err)
		return
	}

	// Recompute content_checksum and bump version
	if err := h.syncService.RebuildSkillBundle(ctx, skillID); err != nil {
		response.InternalError(c, "Failed to recompute checksum", err)
		return
	}

	protobind.OK(c, convert.SkillFileToProto(sf))
}

// GetSkillFileContent reads a text file's content.
func (h *Handler) GetSkillFileContent(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid skill ID", err)
		return
	}

	filepath := c.Query("path")
	if filepath == "" {
		response.BadRequest(c, "path parameter required")
		return
	}

	ctx := context.Background()

	var sf models.SkillFile
	err = h.db.NewSelect().Model(&sf).Where("skill_id = ? AND filepath = ?", skillID, filepath).Scan(ctx)
	if err != nil {
		response.NotFound(c, "File not found", err)
		return
	}

	if h.syncService.storage == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}
	backend := h.syncService.storage.GetClient(ctx)
	if backend == nil {
		response.ServiceUnavailable(c, "Storage not configured")
		return
	}

	body, err := backend.Download(ctx, sf.S3Key)
	if err != nil {
		response.NotFound(c, "File not found in storage", err)
		return
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		response.InternalError(c, "Failed to read file", err)
		return
	}

	protobind.OK(c, &sacv1.SkillFileContentResponse{
		Filepath:    sf.Filepath,
		Content:     string(data),
		ContentType: sf.ContentType,
		Size:        sf.Size,
	})
}
