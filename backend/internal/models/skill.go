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

// SkillFrontmatter holds YAML frontmatter fields for a skill's SKILL.md.
type SkillFrontmatter struct {
	AllowedTools           []string `json:"allowed_tools,omitempty"`
	Model                  string   `json:"model,omitempty"`
	Context                string   `json:"context,omitempty"`
	Agent                  string   `json:"agent,omitempty"`
	DisableModelInvocation bool     `json:"disable_model_invocation,omitempty"`
	ArgumentHint           string   `json:"argument_hint,omitempty"`
	UserInvocable          *bool    `json:"user_invocable,omitempty"`
}

// IsZero returns true if all frontmatter fields are empty/default.
func (f *SkillFrontmatter) IsZero() bool {
	return len(f.AllowedTools) == 0 &&
		f.Model == "" && f.Context == "" && f.Agent == "" &&
		!f.DisableModelInvocation && f.ArgumentHint == "" &&
		f.UserInvocable == nil
}

// Scan implements sql.Scanner for JSONB column.
func (f *SkillFrontmatter) Scan(value any) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, f)
}

// Value implements driver.Valuer for JSONB column.
func (f SkillFrontmatter) Value() (driver.Value, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// SkillFile represents an attached file stored alongside a skill.
type SkillFile struct {
	bun.BaseModel `bun:"table:skill_files,alias:sf"`

	ID          int64     `bun:"id,pk,autoincrement" json:"id"`
	SkillID     int64     `bun:"skill_id,notnull" json:"skill_id"`
	Filepath    string    `bun:"filepath,notnull" json:"filepath"`
	S3Key       string    `bun:"s3_key,notnull" json:"s3_key"`
	Checksum    string    `bun:"checksum,notnull,default:''" json:"checksum"`
	Size        int64     `bun:"size,notnull,default:0" json:"size"`
	ContentType string    `bun:"content_type,notnull,default:'application/octet-stream'" json:"content_type"`
	CreatedAt   time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
}

type Skill struct {
	bun.BaseModel `bun:"table:skills,alias:sk"`

	ID              int64            `bun:"id,pk,autoincrement" json:"id"`
	Name            string           `bun:"name,notnull" json:"name"`
	Description     string           `bun:"description" json:"description"`
	Icon            string           `bun:"icon" json:"icon"`
	Category        string           `bun:"category,notnull" json:"category"`
	Prompt          string           `bun:"prompt,type:text,notnull" json:"prompt"`
	CommandName     string           `bun:"command_name" json:"command_name"`
	Parameters      SkillParameters  `bun:"parameters,type:jsonb" json:"parameters,omitempty"`
	Frontmatter     SkillFrontmatter `bun:"frontmatter,type:jsonb,notnull,default:'{}'" json:"frontmatter"`
	IsOfficial      bool             `bun:"is_official,notnull,default:false" json:"is_official"`
	CreatedBy       int64            `bun:"created_by,notnull" json:"created_by"`
	IsPublic        bool             `bun:"is_public,notnull,default:false" json:"is_public"`
	ForkedFrom      *int64           `bun:"forked_from" json:"forked_from,omitempty"`
	GroupID         *int64           `bun:"group_id" json:"group_id,omitempty"`
	Version         int              `bun:"version,notnull,default:1" json:"version"`
	ContentChecksum string           `bun:"content_checksum,notnull,default:''" json:"content_checksum"`
	CreatedAt       time.Time        `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt       time.Time        `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	Creator *User       `bun:"rel:belongs-to,join:created_by=id" json:"creator,omitempty"`
	Group   *Group      `bun:"rel:belongs-to,join:group_id=id" json:"group,omitempty"`
	Files   []SkillFile `bun:"rel:has-many,join:id=skill_id" json:"files,omitempty"`
}
