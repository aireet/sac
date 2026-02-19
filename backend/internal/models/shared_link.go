package models

import (
	"time"

	"github.com/uptrace/bun"
)

type SharedLink struct {
	bun.BaseModel `bun:"table:shared_links,alias:sl"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	ShortCode string    `bun:"short_code,notnull" json:"short_code"`
	UserID    int64     `bun:"user_id,notnull" json:"user_id"`
	AgentID   int64     `bun:"agent_id,notnull" json:"agent_id"`
	FilePath  string    `bun:"file_path,notnull" json:"file_path"`
	OSSKey    string    `bun:"oss_key,notnull" json:"oss_key"`
	FileName  string    `bun:"file_name,notnull" json:"file_name"`
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
}
