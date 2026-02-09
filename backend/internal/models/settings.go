package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// SettingValue is a custom type for JSONB setting values
type SettingValue json.RawMessage

func (sv *SettingValue) Scan(value any) error {
	if value == nil {
		*sv = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	*sv = SettingValue(bytes)
	return nil
}

func (sv SettingValue) Value() (driver.Value, error) {
	if sv == nil {
		return nil, nil
	}
	return string(sv), nil
}

func (sv SettingValue) MarshalJSON() ([]byte, error) {
	if sv == nil {
		return []byte("null"), nil
	}
	return []byte(sv), nil
}

func (sv *SettingValue) UnmarshalJSON(data []byte) error {
	*sv = SettingValue(data)
	return nil
}

type SystemSetting struct {
	bun.BaseModel `bun:"table:system_settings,alias:ss"`

	ID          int64        `bun:"id,pk,autoincrement" json:"id"`
	Key         string       `bun:"key,notnull,unique" json:"key"`
	Value       SettingValue `bun:"value,type:jsonb,notnull" json:"value"`
	Description string       `bun:"description" json:"description"`
	CreatedAt   time.Time    `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time    `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

type UserSetting struct {
	bun.BaseModel `bun:"table:user_settings,alias:us"`

	ID        int64        `bun:"id,pk,autoincrement" json:"id"`
	UserID    int64        `bun:"user_id,notnull" json:"user_id"`
	Key       string       `bun:"key,notnull" json:"key"`
	Value     SettingValue `bun:"value,type:jsonb,notnull" json:"value"`
	CreatedAt time.Time    `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time    `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
