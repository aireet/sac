package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// AgentConfig stores additional agent configuration as JSONB
type AgentConfig map[string]any

// Scan implements sql.Scanner interface for reading from database
func (ac *AgentConfig) Scan(value any) error {
	if value == nil {
		*ac = nil
		return nil
	}

	// Handle both []byte and string
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, ac)
	case string:
		return json.Unmarshal([]byte(v), ac)
	default:
		return nil
	}
}

// Value implements driver.Valuer interface for writing to database
// We return the JSON as a string to avoid PostgreSQL treating it as bytea
func (ac AgentConfig) Value() (driver.Value, error) {
	if len(ac) == 0 {
		return "{}", nil
	}
	bytes, err := json.Marshal(ac)
	if err != nil {
		return nil, err
	}
	// Return as string to ensure PostgreSQL treats it as JSON/JSONB, not bytea
	return string(bytes), nil
}

type Agent struct {
	bun.BaseModel `bun:"table:agents,alias:ag"`

	ID            int64       `bun:"id,pk,autoincrement" json:"id"`
	Name          string      `bun:"name,notnull" json:"name"`
	Description   string      `bun:"description" json:"description"`
	Icon          string      `bun:"icon" json:"icon"`
	Config        AgentConfig `bun:"config,type:jsonb" json:"config,omitempty"`
	CreatedBy     int64       `bun:"created_by,notnull" json:"created_by"`
	CPURequest    *string     `bun:"cpu_request" json:"cpu_request"`
	CPULimit      *string     `bun:"cpu_limit" json:"cpu_limit"`
	MemoryRequest *string     `bun:"memory_request" json:"memory_request"`
	MemoryLimit   *string     `bun:"memory_limit" json:"memory_limit"`
	Instructions  string      `bun:"instructions" json:"instructions"`
	CreatedAt     time.Time   `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time   `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	Creator         *User        `bun:"rel:belongs-to,join:created_by=id" json:"creator,omitempty"`
	InstalledSkills []AgentSkill `bun:"rel:has-many,join:id=agent_id" json:"installed_skills,omitempty"`
}

// AgentSkill represents the many-to-many relationship between agents and skills
type AgentSkill struct {
	bun.BaseModel `bun:"table:agent_skills,alias:as"`

	ID            int64     `bun:"id,pk,autoincrement" json:"id"`
	AgentID       int64     `bun:"agent_id,notnull" json:"agent_id"`
	SkillID       int64     `bun:"skill_id,notnull" json:"skill_id"`
	Order         int       `bun:"order,notnull,default:0" json:"order"` // Display order
	SyncedVersion int       `bun:"synced_version,notnull,default:0" json:"synced_version"`
	CreatedAt     time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`

	// Relations
	Agent *Agent `bun:"rel:belongs-to,join:agent_id=id" json:"agent,omitempty"`
	Skill *Skill `bun:"rel:belongs-to,join:skill_id=id" json:"skill,omitempty"`
}
