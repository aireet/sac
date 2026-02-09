package models

import (
	"time"

	"github.com/uptrace/bun"
)

type ConversationHistory struct {
	bun.BaseModel `bun:"table:conversation_histories,alias:ch"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	UserID      int64     `bun:"user_id,notnull" json:"user_id"`
	AgentID     int64     `bun:"agent_id,notnull" json:"agent_id"`
	SessionID   string    `bun:"session_id,notnull" json:"session_id"`
	Role        string    `bun:"role,notnull" json:"role"`
	Content     string    `bun:"content,type:text,notnull" json:"content"`
	MessageUUID string    `bun:"message_uuid" json:"message_uuid,omitempty"`
	Timestamp   time.Time `bun:"timestamp,notnull,default:current_timestamp" json:"timestamp"`
}
