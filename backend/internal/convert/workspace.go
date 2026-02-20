package convert

import (
	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func WorkspaceFileToProto(m *models.WorkspaceFile) *sacv1.WorkspaceFile {
	pb := &sacv1.WorkspaceFile{
		Id:            m.ID,
		UserId:        m.UserID,
		AgentId:       m.AgentID,
		GroupId:       m.GroupID,
		WorkspaceType: m.WorkspaceType,
		OssKey:        m.OSSKey,
		FileName:      m.FileName,
		FilePath:      m.FilePath,
		ContentType:   m.ContentType,
		SizeBytes:     m.SizeBytes,
		Checksum:      m.Checksum,
		IsDirectory:   m.IsDirectory,
		CreatedAt:     timestamppb.New(m.CreatedAt),
		UpdatedAt:     timestamppb.New(m.UpdatedAt),
	}
	return pb
}

func WorkspaceQuotaToProto(m *models.WorkspaceQuota) *sacv1.WorkspaceQuota {
	return &sacv1.WorkspaceQuota{
		UserId:       m.UserID,
		AgentId:      m.AgentID,
		UsedBytes:    m.UsedBytes,
		MaxBytes:     m.MaxBytes,
		FileCount:    int32(m.FileCount),
		MaxFileCount: int32(m.MaxFileCount),
	}
}

func GroupWorkspaceQuotaToProto(m *models.GroupWorkspaceQuota) *sacv1.GroupWorkspaceQuota {
	return &sacv1.GroupWorkspaceQuota{
		GroupId:      m.GroupID,
		UsedBytes:    m.UsedBytes,
		MaxBytes:     m.MaxBytes,
		FileCount:    int32(m.FileCount),
		MaxFileCount: int32(m.MaxFileCount),
	}
}
