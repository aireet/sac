package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

type SkillParameter struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Type         string   `json:"type"` // text, select, date, number
	Required     bool     `json:"required"`
	DefaultValue string   `json:"default_value,omitempty"`
	Options      []string `json:"options,omitempty"` // for select type
}

// SkillParameters is a custom type for JSON serialization
type SkillParameters []SkillParameter

// Scan implements sql.Scanner interface
func (sp *SkillParameters) Scan(value any) error {
	if value == nil {
		*sp = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, sp)
}

// Value implements driver.Valuer interface
func (sp SkillParameters) Value() (driver.Value, error) {
	if sp == nil {
		return nil, nil
	}
	b, err := json.Marshal(sp)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

type Skill struct {
	bun.BaseModel `bun:"table:skills,alias:sk"`

	ID          int64           `bun:"id,pk,autoincrement" json:"id"`
	Name        string          `bun:"name,notnull" json:"name"`
	Description string          `bun:"description" json:"description"`
	Icon        string          `bun:"icon" json:"icon"`
	Category    string          `bun:"category,notnull" json:"category"`
	Prompt      string          `bun:"prompt,type:text,notnull" json:"prompt"`
	CommandName string          `bun:"command_name" json:"command_name"`
	Parameters  SkillParameters `bun:"parameters,type:jsonb" json:"parameters,omitempty"`
	IsOfficial  bool            `bun:"is_official,notnull,default:false" json:"is_official"`
	CreatedBy   int64           `bun:"created_by,notnull" json:"created_by"`
	IsPublic    bool            `bun:"is_public,notnull,default:false" json:"is_public"`
	ForkedFrom  *int64          `bun:"forked_from" json:"forked_from,omitempty"`
	Version     int             `bun:"version,notnull,default:1" json:"version"`
	CreatedAt   time.Time       `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time       `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	Creator *User `bun:"rel:belongs-to,join:created_by=id" json:"creator,omitempty"`
}
