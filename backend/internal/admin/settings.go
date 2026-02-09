package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

type ResourceConfig struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
}

type SettingsService struct {
	db *bun.DB
}

func NewSettingsService(db *bun.DB) *SettingsService {
	return &SettingsService{db: db}
}

// GetSetting returns the value of a system setting.
func (s *SettingsService) GetSetting(ctx context.Context, key string) (string, error) {
	var setting models.SystemSetting
	err := s.db.NewSelect().Model(&setting).Where("key = ?", key).Scan(ctx)
	if err != nil {
		return "", err
	}
	// Value is JSONB, so it's a JSON string (e.g. `"3"` or `"2Gi"`)
	var val string
	if err := json.Unmarshal([]byte(setting.Value), &val); err != nil {
		return string(setting.Value), nil
	}
	return val, nil
}

// GetUserSetting returns the value for a specific user, falling back to the system default.
func (s *SettingsService) GetUserSetting(ctx context.Context, userID int64, key string) (string, error) {
	// Check user-specific override first
	var userSetting models.UserSetting
	err := s.db.NewSelect().Model(&userSetting).
		Where("user_id = ? AND key = ?", userID, key).
		Scan(ctx)
	if err == nil {
		var val string
		if err := json.Unmarshal([]byte(userSetting.Value), &val); err != nil {
			return string(userSetting.Value), nil
		}
		return val, nil
	}
	// Fall back to system default
	return s.GetSetting(ctx, key)
}

// GetMaxAgents returns the max agents allowed for a user.
func (s *SettingsService) GetMaxAgents(ctx context.Context, userID int64) (int, error) {
	val, err := s.GetUserSetting(ctx, userID, "max_agents_per_user")
	if err != nil {
		return 3, nil // default
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 3, nil
	}
	return n, nil
}

// GetResourceLimits returns the resource configuration for a user.
func (s *SettingsService) GetResourceLimits(ctx context.Context, userID int64) ResourceConfig {
	cpuReq, _ := s.GetUserSetting(ctx, userID, "default_cpu_request")
	cpuLim, _ := s.GetUserSetting(ctx, userID, "default_cpu_limit")
	memReq, _ := s.GetUserSetting(ctx, userID, "default_memory_request")
	memLim, _ := s.GetUserSetting(ctx, userID, "default_memory_limit")

	rc := ResourceConfig{
		CPURequest:    "2",
		CPULimit:      "2",
		MemoryRequest: "4Gi",
		MemoryLimit:   "4Gi",
	}
	if cpuReq != "" {
		rc.CPURequest = cpuReq
	}
	if cpuLim != "" {
		rc.CPULimit = cpuLim
	}
	if memReq != "" {
		rc.MemoryRequest = memReq
	}
	if memLim != "" {
		rc.MemoryLimit = memLim
	}
	return rc
}
