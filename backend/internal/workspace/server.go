package workspace

import (
	"context"
	"path"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Server implements WorkspaceServiceServer for output and sharing operations.
type Server struct {
	sacv1.UnimplementedWorkspaceServiceServer
	db       *bun.DB
	provider *storage.StorageProvider
	hub      *OutputHub
}

func NewWorkspaceServer(db *bun.DB, provider *storage.StorageProvider, hub *OutputHub) *Server {
	return &Server{db: db, provider: provider, hub: hub}
}

func (s *Server) getOSS(ctx context.Context) (storage.StorageBackend, error) {
	backend := s.provider.GetClient(ctx)
	if backend == nil {
		return nil, grpcerr.Unavailable("Workspace storage is not configured. Ask your admin to configure storage settings.")
	}
	return backend, nil
}

func (s *Server) GetStatus(ctx context.Context, _ *sacv1.Empty) (*sacv1.WorkspaceStatusResponse, error) {
	configured := s.provider.IsConfigured(ctx)
	return &sacv1.WorkspaceStatusResponse{Configured: configured}, nil
}

// --- Output workspace ---

func (s *Server) ListOutputFiles(ctx context.Context, req *sacv1.ListOutputFilesRequest) (*sacv1.FileListResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	reqPath := sanitizePath(req.Path)
	prefix := outputOSSKeyPrefix(userID, req.AgentId) + reqPath

	items, err := oss.List(ctx, prefix, "/", 1000)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list output files", err)
	}

	basePrefix := outputOSSKeyPrefix(userID, req.AgentId)
	return &sacv1.FileListResponse{Path: reqPath, Files: storageItemsToProto(items, basePrefix)}, nil
}

func (s *Server) DeleteOutputFile(ctx context.Context, req *sacv1.DeleteOutputFileRequest) (*sacv1.SuccessMessage, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if req.Path == "" {
		return nil, grpcerr.BadRequest("path is required")
	}
	filePath := sanitizePath(req.Path)
	ossKey := outputOSSKeyPrefix(userID, req.AgentId) + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete directory", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'output' AND oss_key LIKE ?", userID, req.AgentId, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete file", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	_, _ = s.db.NewDelete().Model((*models.SharedLink)(nil)).
		Where("user_id = ? AND agent_id = ? AND file_path = ?", userID, req.AgentId, filePath).
		Exec(ctx)

	if s.hub != nil {
		s.hub.Publish(ctx, userID, req.AgentId, OutputEvent{
			Action: "delete",
			Path:   filePath,
			Name:   path.Base(filePath),
		})
	}

	return &sacv1.SuccessMessage{Message: "File deleted"}, nil
}

// --- Sharing ---

func (s *Server) CreateShare(ctx context.Context, req *sacv1.CreateShareRequest) (*sacv1.ShareResponse, error) {
	userID := ctxkeys.UserID(ctx)

	if req.AgentId == 0 || req.Path == "" {
		return nil, grpcerr.BadRequest("agent_id and path are required")
	}

	filePath := sanitizePath(req.Path)
	ossKey := outputOSSKeyPrefix(userID, req.AgentId) + filePath

	var existing models.SharedLink
	err := s.db.NewSelect().Model(&existing).
		Where("user_id = ? AND agent_id = ? AND file_path = ?", userID, req.AgentId, filePath).
		Scan(ctx)
	if err == nil {
		return &sacv1.ShareResponse{ShortCode: existing.ShortCode, Url: "/s/" + existing.ShortCode}, nil
	}

	backend := s.provider.GetClient(ctx)
	if backend == nil {
		return nil, grpcerr.Unavailable("Storage not configured")
	}
	body, err := backend.Download(ctx, ossKey)
	if err != nil {
		return nil, grpcerr.NotFound("File not found in output workspace", err)
	}
	body.Close()

	shortCode := uuid.New().String()[:8]
	fileName := path.Base(filePath)

	link := &models.SharedLink{
		ShortCode: shortCode,
		UserID:    userID,
		AgentID:   req.AgentId,
		FilePath:  filePath,
		OSSKey:    ossKey,
		FileName:  fileName,
		CreatedAt: time.Now(),
	}

	_, err = s.db.NewInsert().Model(link).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create share link", err)
	}

	return &sacv1.ShareResponse{ShortCode: shortCode, Url: "/s/" + shortCode}, nil
}

func (s *Server) DeleteShare(ctx context.Context, req *sacv1.DeleteShareRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	if req.Code == "" {
		return nil, grpcerr.BadRequest("code is required")
	}

	result, err := s.db.NewDelete().Model((*models.SharedLink)(nil)).
		Where("short_code = ? AND user_id = ?", req.Code, userID).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete share link", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, grpcerr.NotFound("Share link not found")
	}

	return &sacv1.SuccessMessage{Message: "Share link deleted"}, nil
}

func (s *Server) GetSharedFileMeta(ctx context.Context, req *sacv1.GetSharedFileRequest) (*sacv1.SharedFileMeta, error) {
	if req.Code == "" {
		return nil, grpcerr.BadRequest("code is required")
	}

	var link models.SharedLink
	err := s.db.NewSelect().Model(&link).Where("short_code = ?", req.Code).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Link not found or expired")
	}

	contentType := contentTypeByFilename(link.FileName)

	var sizeBytes int64
	_ = s.db.NewSelect().
		TableExpr("workspace_files").
		Column("size_bytes").
		Where("oss_key = ?", link.OSSKey).
		Scan(ctx, &sizeBytes)

	return &sacv1.SharedFileMeta{
		FileName:    link.FileName,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
	}, nil
}

func (s *Server) InternalOutputDelete(ctx context.Context, req *sacv1.InternalOutputDeleteRequest) (*sacv1.SuccessMessage, error) {
	if req.UserId == 0 || req.AgentId == 0 || req.Path == "" {
		return nil, grpcerr.BadRequest("user_id, agent_id, and path are required")
	}

	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}

	filePath := sanitizePath(req.Path)
	ossKey := outputOSSKeyPrefix(req.UserId, req.AgentId) + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete directory", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'output' AND oss_key LIKE ?", req.UserId, req.AgentId, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete file", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	if s.hub != nil {
		s.hub.Publish(ctx, req.UserId, req.AgentId, OutputEvent{
			Action: "delete",
			Path:   filePath,
			Name:   path.Base(filePath),
		})
	}

	return &sacv1.SuccessMessage{Message: "File deleted"}, nil
}
