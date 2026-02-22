package workspace

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/storage"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Server implements WorkspaceServiceServer for non-file-transfer operations.
type Server struct {
	sacv1.UnimplementedWorkspaceServiceServer
	db       *bun.DB
	provider *storage.StorageProvider
	syncSvc  *SyncService
	hub      *OutputHub
}

func NewWorkspaceServer(db *bun.DB, provider *storage.StorageProvider, syncSvc *SyncService, hub *OutputHub) *Server {
	return &Server{db: db, provider: provider, syncSvc: syncSvc, hub: hub}
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

// --- Private workspace ---

func (s *Server) ListFiles(ctx context.Context, req *sacv1.ListFilesRequest) (*sacv1.FileListResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	reqPath := sanitizePath(req.Path)
	if reqPath == "" {
		reqPath = ""
	}
	prefix := ossKeyPrefix(userID, req.AgentId) + reqPath

	items, err := oss.List(ctx, prefix, "/", 1000)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list files", err)
	}

	basePrefix := ossKeyPrefix(userID, req.AgentId)
	return &sacv1.FileListResponse{Path: reqPath, Files: storageItemsToProto(items, basePrefix)}, nil
}

func (s *Server) DeleteFile(ctx context.Context, req *sacv1.DeleteFileRequest) (*sacv1.SuccessMessage, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if req.Path == "" {
		return nil, grpcerr.BadRequest("path is required")
	}
	filePath := sanitizePath(req.Path)
	ossKey := ossKeyPrefix(userID, req.AgentId) + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete directory", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("user_id = ? AND agent_id = ? AND workspace_type = 'private' AND oss_key LIKE ?", userID, req.AgentId, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete file", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	s.recalcQuota(ctx, userID, req.AgentId)
	go s.syncSvc.DeleteFileFromPods(context.Background(), userID, req.AgentId, filePath)

	return &sacv1.SuccessMessage{Message: "File deleted"}, nil
}

func (s *Server) CreateDirectory(ctx context.Context, req *sacv1.CreateDirectoryRequest) (*sacv1.DirectoryResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if req.AgentId == 0 || req.Path == "" {
		return nil, grpcerr.BadRequest("path and agent_id are required")
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := ossKeyPrefix(userID, req.AgentId) + dirPath
	if err := oss.Upload(ctx, ossKey, strings.NewReader(""), 0, "application/x-directory"); err != nil {
		return nil, grpcerr.Internal("Failed to create directory", err)
	}

	return &sacv1.DirectoryResponse{Path: dirPath}, nil
}

func (s *Server) GetQuota(ctx context.Context, req *sacv1.GetQuotaRequest) (*sacv1.WorkspaceQuota, error) {
	userID := ctxkeys.UserID(ctx)
	quota := s.getOrCreateQuota(ctx, userID, req.AgentId)
	return convert.WorkspaceQuotaToProto(quota), nil
}

// --- Public workspace ---

func (s *Server) ListPublicFiles(ctx context.Context, req *sacv1.ListPublicFilesRequest) (*sacv1.FileListResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}

	reqPath := sanitizePath(req.Path)
	prefix := "public/" + reqPath

	items, err := oss.List(ctx, prefix, "/", 1000)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list public files", err)
	}

	return &sacv1.FileListResponse{Path: reqPath, Files: storageItemsToProto(items, "public/")}, nil
}

func (s *Server) CreatePublicDirectory(ctx context.Context, req *sacv1.CreatePublicDirectoryRequest) (*sacv1.DirectoryResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}

	role := ctxkeys.Role(ctx)
	if role != "admin" {
		return nil, grpcerr.Forbidden("Admin access required")
	}

	if req.Path == "" {
		return nil, grpcerr.BadRequest("path is required")
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := "public/" + dirPath
	if err := oss.Upload(ctx, ossKey, strings.NewReader(""), 0, "application/x-directory"); err != nil {
		return nil, grpcerr.Internal("Failed to create directory", err)
	}

	return &sacv1.DirectoryResponse{Path: dirPath}, nil
}

func (s *Server) DeletePublicFile(ctx context.Context, req *sacv1.DeletePublicFileRequest) (*sacv1.SuccessMessage, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}

	role := ctxkeys.Role(ctx)
	if role != "admin" {
		return nil, grpcerr.Forbidden("Admin access required")
	}

	if req.Path == "" {
		return nil, grpcerr.BadRequest("path is required")
	}
	filePath := sanitizePath(req.Path)
	ossKey := "public/" + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete directory", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("workspace_type = 'public' AND oss_key LIKE ?", ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete file", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	go s.syncSvc.DeletePublicFileFromPods(context.Background(), filePath)

	return &sacv1.SuccessMessage{Message: "File deleted"}, nil
}

// --- Group workspace ---

func (s *Server) ListGroupFiles(ctx context.Context, req *sacv1.ListGroupFilesRequest) (*sacv1.FileListResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	reqPath := sanitizePath(req.Path)
	prefix := groupOSSKeyPrefix(req.GroupId) + reqPath

	items, err := oss.List(ctx, prefix, "/", 1000)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list group files", err)
	}

	return &sacv1.FileListResponse{Path: reqPath, Files: storageItemsToProto(items, groupOSSKeyPrefix(req.GroupId))}, nil
}

func (s *Server) CreateGroupDirectory(ctx context.Context, req *sacv1.CreateGroupDirectoryRequest) (*sacv1.DirectoryResponse, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if req.GroupId == 0 || req.Path == "" {
		return nil, grpcerr.BadRequest("path and group_id are required")
	}

	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	dirPath := sanitizePath(req.Path)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	ossKey := groupOSSKeyPrefix(req.GroupId) + dirPath
	if err := oss.Upload(ctx, ossKey, strings.NewReader(""), 0, "application/x-directory"); err != nil {
		return nil, grpcerr.Internal("Failed to create directory", err)
	}

	return &sacv1.DirectoryResponse{Path: dirPath}, nil
}

func (s *Server) DeleteGroupFile(ctx context.Context, req *sacv1.DeleteGroupFileRequest) (*sacv1.SuccessMessage, error) {
	oss, err := s.getOSS(ctx)
	if err != nil {
		return nil, err
	}
	userID := ctxkeys.UserID(ctx)

	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	if req.Path == "" {
		return nil, grpcerr.BadRequest("path is required")
	}
	filePath := sanitizePath(req.Path)
	ossKey := groupOSSKeyPrefix(req.GroupId) + filePath

	if strings.HasSuffix(filePath, "/") {
		if err := oss.DeletePrefix(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete directory", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("group_id = ? AND workspace_type = 'group' AND oss_key LIKE ?", req.GroupId, ossKey+"%").
			Exec(ctx)
	} else {
		if err := oss.Delete(ctx, ossKey); err != nil {
			return nil, grpcerr.Internal("Failed to delete file", err)
		}
		_, _ = s.db.NewDelete().Model((*models.WorkspaceFile)(nil)).
			Where("oss_key = ?", ossKey).
			Exec(ctx)
	}

	s.recalcGroupQuota(ctx, req.GroupId)
	go s.syncSvc.DeleteGroupFileFromPods(context.Background(), req.GroupId, filePath)

	return &sacv1.SuccessMessage{Message: "File deleted"}, nil
}

func (s *Server) GetGroupQuota(ctx context.Context, req *sacv1.GetGroupQuotaRequest) (*sacv1.GroupWorkspaceQuota, error) {
	userID := ctxkeys.UserID(ctx)

	if !s.isGroupMember(ctx, req.GroupId, userID) {
		return nil, grpcerr.Forbidden("Not a member of this group")
	}

	quota := s.getOrCreateGroupQuota(ctx, req.GroupId)
	return convert.GroupWorkspaceQuotaToProto(quota), nil
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

func (s *Server) SyncToPod(ctx context.Context, req *sacv1.SyncToPodRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)
	userIDStr := fmt.Sprintf("%d", userID)

	if req.AgentId <= 0 {
		return nil, grpcerr.BadRequest("agent_id is required")
	}

	if err := s.syncSvc.SyncWorkspaceToPod(ctx, userIDStr, req.AgentId); err != nil {
		log.Printf("Workspace sync failed for user %s agent %d: %v", userIDStr, req.AgentId, err)
		return nil, grpcerr.Internal("Workspace sync failed", err)
	}

	return &sacv1.SuccessMessage{Message: "Workspace synced to pod"}, nil
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

// --- Helpers ---

func (s *Server) isGroupMember(ctx context.Context, groupID, userID int64) bool {
	exists, _ := s.db.NewSelect().Model((*models.GroupMember)(nil)).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Exists(ctx)
	return exists
}

func (s *Server) getOrCreateQuota(ctx context.Context, userID, agentID int64) *models.WorkspaceQuota {
	var quota models.WorkspaceQuota
	err := s.db.NewSelect().Model(&quota).Where("user_id = ? AND agent_id = ?", userID, agentID).Scan(ctx)
	if err != nil {
		quota = models.WorkspaceQuota{
			UserID:       userID,
			AgentID:      agentID,
			UsedBytes:    0,
			MaxBytes:     1 << 30,
			FileCount:    0,
			MaxFileCount: 1000,
			UpdatedAt:    time.Now(),
		}
		_, _ = s.db.NewInsert().Model(&quota).Exec(ctx)
	}
	return &quota
}

func (s *Server) recalcQuota(ctx context.Context, userID, agentID int64) {
	var result struct {
		TotalSize int64 `bun:"total_size"`
		FileCount int   `bun:"file_count"`
	}

	err := s.db.NewSelect().
		TableExpr("workspace_files").
		ColumnExpr("COALESCE(SUM(size_bytes), 0) AS total_size").
		ColumnExpr("COUNT(*) AS file_count").
		Where("user_id = ? AND agent_id = ? AND workspace_type = 'private' AND is_directory = FALSE", userID, agentID).
		Scan(ctx, &result)
	if err != nil {
		log.Printf("Warning: failed to recalc quota for user %d agent %d: %v", userID, agentID, err)
		return
	}

	_, _ = s.db.NewUpdate().Model((*models.WorkspaceQuota)(nil)).
		Set("used_bytes = ?", result.TotalSize).
		Set("file_count = ?", result.FileCount).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ? AND agent_id = ?", userID, agentID).
		Exec(ctx)
}

func (s *Server) getOrCreateGroupQuota(ctx context.Context, groupID int64) *models.GroupWorkspaceQuota {
	var quota models.GroupWorkspaceQuota
	err := s.db.NewSelect().Model(&quota).Where("group_id = ?", groupID).Scan(ctx)
	if err != nil {
		quota = models.GroupWorkspaceQuota{
			GroupID:      groupID,
			UsedBytes:    0,
			MaxBytes:     1 << 30,
			FileCount:    0,
			MaxFileCount: 1000,
			UpdatedAt:    time.Now(),
		}
		_, _ = s.db.NewInsert().Model(&quota).Exec(ctx)
	}
	return &quota
}

func (s *Server) recalcGroupQuota(ctx context.Context, groupID int64) {
	var result struct {
		TotalSize int64 `bun:"total_size"`
		FileCount int   `bun:"file_count"`
	}

	err := s.db.NewSelect().
		TableExpr("workspace_files").
		ColumnExpr("COALESCE(SUM(size_bytes), 0) AS total_size").
		ColumnExpr("COUNT(*) AS file_count").
		Where("group_id = ? AND workspace_type = 'group' AND is_directory = FALSE", groupID).
		Scan(ctx, &result)
	if err != nil {
		log.Printf("Warning: failed to recalc group quota for group %d: %v", groupID, err)
		return
	}

	_, _ = s.db.NewUpdate().Model((*models.GroupWorkspaceQuota)(nil)).
		Set("used_bytes = ?", result.TotalSize).
		Set("file_count = ?", result.FileCount).
		Set("updated_at = ?", time.Now()).
		Where("group_id = ?", groupID).
		Exec(ctx)
}
