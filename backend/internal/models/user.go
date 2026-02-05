package models

import (
	"time"

	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	Username    string    `bun:"username,notnull,unique" json:"username"`
	Email       string    `bun:"email,notnull,unique" json:"email"`
	DisplayName string    `bun:"display_name" json:"display_name"`
	CreatedAt   time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
