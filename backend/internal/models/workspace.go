package models

import (
	"time"

	"github.com/uptrace/bun"
)

type WorkspaceFile struct {
	bun.BaseModel `bun:"table:workspace_files,alias:wf"`

	ID            int64     `bun:"id,pk,autoincrement" json:"id"`
	UserID        int64     `bun:"user_id,notnull" json:"user_id"`
	AgentID       int64     `bun:"agent_id,notnull,default:0" json:"agent_id"`
	GroupID       *int64    `bun:"group_id" json:"group_id,omitempty"`
	WorkspaceType string    `bun:"workspace_type,notnull" json:"workspace_type"` // "private" | "public" | "group" | "shared"
	OSSKey        string    `bun:"oss_key,notnull" json:"oss_key"`
	FileName      string    `bun:"file_name,notnull" json:"file_name"`
	FilePath      string    `bun:"file_path,notnull" json:"file_path"` // relative path within workspace
	ContentType   string    `bun:"content_type" json:"content_type"`
	SizeBytes     int64     `bun:"size_bytes,notnull,default:0" json:"size_bytes"`
	Checksum      string    `bun:"checksum" json:"checksum,omitempty"`
	IsDirectory   bool      `bun:"is_directory,notnull,default:false" json:"is_directory"`
	CreatedAt     time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

type WorkspaceQuota struct {
	bun.BaseModel `bun:"table:workspace_quotas,alias:wq"`

	UserID       int64     `bun:"user_id,pk" json:"user_id"`
	AgentID      int64     `bun:"agent_id,pk" json:"agent_id"`
	UsedBytes    int64     `bun:"used_bytes,notnull,default:0" json:"used_bytes"`
	MaxBytes     int64     `bun:"max_bytes,notnull,default:1073741824" json:"max_bytes"` // 1GB
	FileCount    int       `bun:"file_count,notnull,default:0" json:"file_count"`
	MaxFileCount int       `bun:"max_file_count,notnull,default:1000" json:"max_file_count"`
	UpdatedAt    time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
