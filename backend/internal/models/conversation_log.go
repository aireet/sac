package models

import (
	"time"

	"github.com/uptrace/bun"
)

type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
)

type ConversationLog struct {
	bun.BaseModel `bun:"table:conversation_logs,alias:cl"`

	ID        int64       `bun:"id,pk,autoincrement" json:"id"`
	UserID    int64       `bun:"user_id,notnull" json:"user_id"`
	SessionID string      `bun:"session_id,notnull" json:"session_id"`
	Type      MessageType `bun:"type,notnull" json:"type"`
	Content   string      `bun:"content,type:text,notnull" json:"content"`
	Timestamp time.Time   `bun:"timestamp,nullzero,notnull,default:current_timestamp" json:"timestamp"`

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}
