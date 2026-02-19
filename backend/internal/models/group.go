package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Group struct {
	bun.BaseModel `bun:"table:groups,alias:g"`

	ID               int64     `bun:"id,pk,autoincrement" json:"id"`
	Name             string    `bun:"name,notnull,unique" json:"name"`
	Description      string    `bun:"description,notnull,default:''" json:"description"`
	OwnerID          int64     `bun:"owner_id,notnull" json:"owner_id"`
	ClaudeMDTemplate string    `bun:"claude_md_template,notnull,default:''" json:"claude_md_template"`
	CreatedAt        time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt        time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations (not stored in DB)
	Owner   *User          `bun:"rel:belongs-to,join:owner_id=id" json:"owner,omitempty"`
	Members []*GroupMember `bun:"rel:has-many,join:id=group_id" json:"members,omitempty"`
}

type GroupMember struct {
	bun.BaseModel `bun:"table:group_members,alias:gm"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	GroupID   int64     `bun:"group_id,notnull" json:"group_id"`
	UserID    int64     `bun:"user_id,notnull" json:"user_id"`
	Role      string    `bun:"role,notnull,default:'member'" json:"role"` // "admin" | "member"
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`

	// Relations
	User  *User  `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
	Group *Group `bun:"rel:belongs-to,join:group_id=id" json:"group,omitempty"`
}

type GroupWorkspaceQuota struct {
	bun.BaseModel `bun:"table:group_workspace_quotas,alias:gwq"`

	GroupID      int64     `bun:"group_id,pk" json:"group_id"`
	UsedBytes    int64     `bun:"used_bytes,notnull,default:0" json:"used_bytes"`
	MaxBytes     int64     `bun:"max_bytes,notnull,default:1073741824" json:"max_bytes"` // 1GB
	FileCount    int       `bun:"file_count,notnull,default:0" json:"file_count"`
	MaxFileCount int       `bun:"max_file_count,notnull,default:1000" json:"max_file_count"`
	UpdatedAt    time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
