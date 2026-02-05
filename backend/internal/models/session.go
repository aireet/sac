package models

import (
	"time"

	"github.com/uptrace/bun"
)

type SessionStatus string

const (
	SessionStatusCreating SessionStatus = "creating"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusIdle     SessionStatus = "idle"
	SessionStatusStopped  SessionStatus = "stopped"
	SessionStatusDeleted  SessionStatus = "deleted"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:s"`

	ID         int64         `bun:"id,pk,autoincrement" json:"id"`
	UserID     int64         `bun:"user_id,notnull" json:"user_id"`
	SessionID  string        `bun:"session_id,notnull,unique" json:"session_id"`
	PodName    string        `bun:"pod_name" json:"pod_name"`
	PodIP      string        `bun:"pod_ip" json:"pod_ip"`
	Status     SessionStatus `bun:"status,notnull" json:"status"`
	LastActive time.Time     `bun:"last_active,nullzero" json:"last_active"`
	CreatedAt  time.Time     `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time     `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}
