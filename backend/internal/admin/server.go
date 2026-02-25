package admin

import (
	"context"
	"fmt"
"github.com/rs/zerolog/log"
	"os"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Server struct {
	sacv1.UnimplementedAdminServiceServer
	db               *bun.DB
	containerManager *container.Manager
	maintenanceImage string
}

func NewServer2(db *bun.DB, cm *container.Manager, maintenanceImage string) *Server {
	return &Server{db: db, containerManager: cm, maintenanceImage: maintenanceImage}
}

func (s *Server) GetSettings(ctx context.Context, _ *sacv1.Empty) (*sacv1.SystemSettingListResponse, error) {
	var settings []models.SystemSetting
	err := s.db.NewSelect().Model(&settings).Order("key ASC").Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch settings", err)
	}
	return &sacv1.SystemSettingListResponse{Settings: convert.SystemSettingsToProto(settings)}, nil
}

func (s *Server) UpdateSetting(ctx context.Context, req *sacv1.UpdateSettingByKeyRequest) (*sacv1.SuccessMessage, error) {
	if req.Value == nil {
		return nil, grpcerr.BadRequest("value is required")
	}

	q := s.db.NewUpdate().Model((*models.SystemSetting)(nil)).
		Set("value = ?", convert.ProtoValueToSettingValue(req.Value)).
		Set("updated_at = ?", time.Now()).
		Where("key = ?", req.Key)

	if req.Description != nil {
		q = q.Set("description = ?", *req.Description)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update setting", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, grpcerr.NotFound("Setting not found")
	}

	// Reconcile CronJob when relevant settings change
	if req.Key == "skill_sync_interval" || req.Key == "conversation_retention_days" {
		go s.ReconcileMaintenanceCronJob(context.Background())
	}

	return &sacv1.SuccessMessage{Message: "Setting updated"}, nil
}

func (s *Server) GetUsers(ctx context.Context, _ *sacv1.Empty) (*sacv1.AdminUserListResponse, error) {
	type UserWithCount struct {
		models.User
		AgentCount int `bun:"agent_count"`
	}

	type memberRow struct {
		UserID    int64  `bun:"user_id"`
		GroupID   int64  `bun:"group_id"`
		GroupName string `bun:"group_name"`
		Role      string `bun:"role"`
	}

	var users []UserWithCount
	err := s.db.NewSelect().
		TableExpr("users AS u").
		ColumnExpr("u.*").
		ColumnExpr("(SELECT COUNT(*) FROM agents WHERE created_by = u.id) AS agent_count").
		Order("u.id ASC").
		Scan(ctx, &users)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch users", err)
	}

	groupMap := make(map[int64][]memberRow)
	if len(users) > 0 {
		userIDs := make([]int64, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
		}

		var rows []memberRow
		_ = s.db.NewSelect().
			TableExpr("group_members AS gm").
			ColumnExpr("gm.user_id").
			ColumnExpr("gm.group_id").
			ColumnExpr("g.name AS group_name").
			ColumnExpr("gm.role").
			Join("JOIN groups AS g ON g.id = gm.group_id").
			Where("gm.user_id IN (?)", bun.In(userIDs)).
			Scan(ctx, &rows)

		for _, r := range rows {
			groupMap[r.UserID] = append(groupMap[r.UserID], r)
		}
	}

	result := make([]*sacv1.AdminUser, len(users))
	for i, u := range users {
		gRows := groupMap[u.ID]
		groups := make([]*sacv1.AdminGroupBrief, len(gRows))
		for j, g := range gRows {
			groups[j] = &sacv1.AdminGroupBrief{Id: g.GroupID, Name: g.GroupName, Role: g.Role}
		}
		result[i] = &sacv1.AdminUser{
			Id:          u.ID,
			Username:    u.Username,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Role:        u.Role,
			AgentCount:  int32(u.AgentCount),
			Groups:      groups,
			CreatedAt:   timestamppb.New(u.CreatedAt),
			UpdatedAt:   timestamppb.New(u.UpdatedAt),
		}
	}

	return &sacv1.AdminUserListResponse{Users: result}, nil
}

func (s *Server) UpdateUserRole(ctx context.Context, req *sacv1.UpdateUserRoleByIdRequest) (*sacv1.SuccessMessage, error) {
	if req.Role != "user" && req.Role != "admin" {
		return nil, grpcerr.BadRequest("role must be 'user' or 'admin'")
	}

	res, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("role = ?", req.Role).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", req.UserId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update role", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, grpcerr.NotFound("User not found")
	}

	return &sacv1.SuccessMessage{Message: "Role updated"}, nil
}

func (s *Server) GetUserSettings(ctx context.Context, req *sacv1.GetUserSettingsRequest) (*sacv1.UserSettingListResponse, error) {
	var settings []models.UserSetting
	err := s.db.NewSelect().Model(&settings).
		Where("user_id = ?", req.UserId).
		Order("key ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch user settings", err)
	}

	if settings == nil {
		settings = []models.UserSetting{}
	}
	return &sacv1.UserSettingListResponse{Settings: convert.UserSettingsToProto(settings)}, nil
}

func (s *Server) SetUserSetting(ctx context.Context, req *sacv1.SetUserSettingByIdRequest) (*sacv1.SuccessMessage, error) {
	if req.Value == nil {
		return nil, grpcerr.BadRequest("value is required")
	}

	setting := &models.UserSetting{
		UserID:    req.UserId,
		Key:       req.Key,
		Value:     convert.ProtoValueToSettingValue(req.Value),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := s.db.NewInsert().Model(setting).
		On("CONFLICT (user_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to set user setting", err)
	}

	return &sacv1.SuccessMessage{Message: "User setting updated"}, nil
}

func (s *Server) DeleteUserSetting(ctx context.Context, req *sacv1.DeleteUserSettingRequest) (*sacv1.SuccessMessage, error) {
	_, err := s.db.NewDelete().Model((*models.UserSetting)(nil)).
		Where("user_id = ? AND key = ?", req.UserId, req.Key).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete user setting", err)
	}

	return &sacv1.SuccessMessage{Message: "User setting deleted"}, nil
}

func (s *Server) GetUserAgents(ctx context.Context, req *sacv1.GetUserAgentsRequest) (*sacv1.AgentWithStatusListResponse, error) {
	userIDStr := fmt.Sprintf("%d", req.UserId)

	var agents []models.Agent
	err := s.db.NewSelect().
		Model(&agents).
		Relation("InstalledSkills").
		Relation("InstalledSkills.Skill").
		Where("ag.created_by = ?", req.UserId).
		Order("ag.id ASC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch agents", err)
	}

	result := make([]*sacv1.AgentWithStatus, 0, len(agents))
	for _, a := range agents {
		info := s.containerManager.GetStatefulSetPodInfo(ctx, userIDStr, a.ID)
		result = append(result, &sacv1.AgentWithStatus{
			Agent:         convert.AgentToProto(&a),
			PodStatus:     info.Status,
			RestartCount:  info.RestartCount,
			CpuRequest:    info.CPURequest,
			CpuLimit:      info.CPULimit,
			MemoryRequest: info.MemoryRequest,
			MemoryLimit:   info.MemoryLimit,
			Image:         info.Image,
		})
	}

	return &sacv1.AgentWithStatusListResponse{Agents: result}, nil
}

func (s *Server) DeleteUserAgent(ctx context.Context, req *sacv1.AdminAgentRequest) (*sacv1.SuccessMessage, error) {
	userIDStr := fmt.Sprintf("%d", req.UserId)

	res, err := s.db.NewDelete().
		Model((*models.Agent)(nil)).
		Where("id = ? AND created_by = ?", req.AgentId, req.UserId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete agent", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, grpcerr.NotFound("Agent not found")
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", req.AgentId).
		Where("user_id = ?", req.UserId).
		Exec(ctx)

	if err := s.containerManager.DeleteStatefulSet(ctx, userIDStr, req.AgentId); err != nil {
		log.Warn().Err(err).Int64("agent_id", req.AgentId).Msg("failed to delete StatefulSet")
	}

	return &sacv1.SuccessMessage{Message: "Agent deleted successfully"}, nil
}

func (s *Server) RestartUserAgent(ctx context.Context, req *sacv1.AdminAgentRequest) (*sacv1.SuccessMessage, error) {
	userIDStr := fmt.Sprintf("%d", req.UserId)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.AgentId, req.UserId).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", req.AgentId).
		Where("user_id = ?", req.UserId).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	if err := s.containerManager.DeleteStatefulSet(ctx, userIDStr, req.AgentId); err != nil {
		return nil, grpcerr.Internal("Failed to restart agent", err)
	}

	log.Info().Int64("agent_id", req.AgentId).Int64("user_id", req.UserId).Msg("admin restarted agent")
	return &sacv1.SuccessMessage{Message: "Agent is restarting"}, nil
}

func (s *Server) UpdateAgentResources(ctx context.Context, req *sacv1.UpdateAgentResourcesByIdRequest) (*sacv1.SuccessMessage, error) {
	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.AgentId, req.UserId).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found")
	}

	q := s.db.NewUpdate().Model((*models.Agent)(nil)).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", req.AgentId)

	if req.CpuRequest != nil {
		if *req.CpuRequest == "" {
			q = q.Set("cpu_request = NULL")
		} else {
			q = q.Set("cpu_request = ?", *req.CpuRequest)
		}
	}
	if req.CpuLimit != nil {
		if *req.CpuLimit == "" {
			q = q.Set("cpu_limit = NULL")
		} else {
			q = q.Set("cpu_limit = ?", *req.CpuLimit)
		}
	}
	if req.MemoryRequest != nil {
		if *req.MemoryRequest == "" {
			q = q.Set("memory_request = NULL")
		} else {
			q = q.Set("memory_request = ?", *req.MemoryRequest)
		}
	}
	if req.MemoryLimit != nil {
		if *req.MemoryLimit == "" {
			q = q.Set("memory_limit = NULL")
		} else {
			q = q.Set("memory_limit = ?", *req.MemoryLimit)
		}
	}

	_, err = q.Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update agent resources", err)
	}

	return &sacv1.SuccessMessage{Message: "Agent resources updated. Restart agent to apply."}, nil
}

func (s *Server) UpdateAgentImage(ctx context.Context, req *sacv1.UpdateAgentImageByIdRequest) (*sacv1.SuccessMessage, error) {
	if req.Image == "" {
		return nil, grpcerr.BadRequest("image is required")
	}

	userIDStr := fmt.Sprintf("%d", req.UserId)

	var agent models.Agent
	err := s.db.NewSelect().Model(&agent).
		Where("id = ? AND created_by = ?", req.AgentId, req.UserId).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found")
	}

	if err := s.containerManager.UpdateStatefulSetImage(ctx, userIDStr, req.AgentId, req.Image); err != nil {
		return nil, grpcerr.Internal("Failed to update agent image", err)
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("agent_id = ?", req.AgentId).
		Where("user_id = ?", req.UserId).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	log.Info().Int64("agent_id", req.AgentId).Str("image", req.Image).Int64("user_id", req.UserId).Msg("admin updated agent image")
	return &sacv1.SuccessMessage{Message: "Agent image updated"}, nil
}

func (s *Server) BatchUpdateImage(ctx context.Context, req *sacv1.BatchUpdateImageRequest) (*sacv1.BatchUpdateImageResponse, error) {
	if req.Image == "" {
		return nil, grpcerr.BadRequest("image is required")
	}

	stsList, err := s.containerManager.ListStatefulSets(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list StatefulSets", err)
	}

	type updateError struct {
		Name  string
		Error string
	}

	var updated int
	var failed int
	var errors []updateError

	for i := range stsList.Items {
		sts := &stsList.Items[i]
		for j := range sts.Spec.Template.Spec.Containers {
			if sts.Spec.Template.Spec.Containers[j].Name == "claude-code" {
				sts.Spec.Template.Spec.Containers[j].Image = req.Image
				break
			}
		}
		_, err := s.containerManager.GetClientset().AppsV1().StatefulSets(sts.Namespace).Update(ctx, sts, metav1.UpdateOptions{})
		if err != nil {
			failed++
			errors = append(errors, updateError{Name: sts.Name, Error: err.Error()})
		} else {
			updated++
		}
	}

	_, _ = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("status IN (?)", bun.In([]string{
			string(models.SessionStatusRunning),
			string(models.SessionStatusCreating),
			string(models.SessionStatusIdle),
		})).
		Exec(ctx)

	batchErrors := make([]*sacv1.BatchUpdateError, len(errors))
	for i, e := range errors {
		batchErrors[i] = &sacv1.BatchUpdateError{Name: e.Name, Error: e.Error}
	}

	return &sacv1.BatchUpdateImageResponse{
		Total:   int32(len(stsList.Items)),
		Updated: int32(updated),
		Failed:  int32(failed),
		Errors:  batchErrors,
	}, nil
}

func (s *Server) ResetUserPassword(ctx context.Context, req *sacv1.ResetPasswordByIdRequest) (*sacv1.SuccessMessage, error) {
	if len(req.NewPassword) < 6 {
		return nil, grpcerr.BadRequest("password must be at least 6 characters")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, grpcerr.Internal("Failed to hash password", err)
	}

	res, err := s.db.NewUpdate().Model((*models.User)(nil)).
		Set("password_hash = ?", string(hashed)).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", req.UserId).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to reset password", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, grpcerr.NotFound("User not found")
	}

	log.Info().Int64("user_id", req.UserId).Msg("admin reset password")
	return &sacv1.SuccessMessage{Message: "Password reset successfully"}, nil
}

func (s *Server) GetConversations(ctx context.Context, req *sacv1.AdminGetConversationsRequest) (*sacv1.AdminConversationListResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	type ConversationRow struct {
		models.ConversationHistory
		Username  string `bun:"username"`
		AgentName string `bun:"agent_name"`
	}

	q := s.db.NewSelect().
		TableExpr("conversation_histories AS ch").
		ColumnExpr("ch.*").
		ColumnExpr("u.username AS username").
		ColumnExpr("a.name AS agent_name").
		Join("LEFT JOIN users AS u ON u.id = ch.user_id").
		Join("LEFT JOIN agents AS a ON a.id = ch.agent_id").
		OrderExpr("ch.timestamp DESC").
		Limit(limit)

	if req.UserId != 0 {
		q = q.Where("ch.user_id = ?", req.UserId)
	}
	if req.AgentId != 0 {
		q = q.Where("ch.agent_id = ?", req.AgentId)
	}
	if req.SessionId != "" {
		q = q.Where("ch.session_id = ?", req.SessionId)
	}
	if req.Before != "" {
		if t, err := time.Parse(time.RFC3339Nano, req.Before); err == nil {
			q = q.Where("ch.timestamp < ?", t)
		}
	}
	if req.Start != "" {
		if t, err := time.Parse(time.RFC3339Nano, req.Start); err == nil {
			q = q.Where("ch.timestamp >= ?", t)
		}
	}
	if req.End != "" {
		if t, err := time.Parse(time.RFC3339Nano, req.End); err == nil {
			q = q.Where("ch.timestamp <= ?", t)
		}
	}

	var rows []ConversationRow
	err := q.Scan(ctx, &rows)
	if err != nil {
		return nil, grpcerr.Internal("Failed to fetch conversations", err)
	}

	convos := make([]*sacv1.AdminConversation, len(rows))
	for i, r := range rows {
		convos[i] = &sacv1.AdminConversation{
			Id:        r.ID,
			UserId:    r.UserID,
			AgentId:   r.AgentID,
			SessionId: r.SessionID,
			Role:      r.Role,
			Content:   r.Content,
			Timestamp: timestamppb.New(r.Timestamp),
			Username:  r.Username,
			AgentName: r.AgentName,
		}
	}

	return &sacv1.AdminConversationListResponse{
		Conversations: convos,
		Count:         int32(len(rows)),
	}, nil
}

// maintenanceEnvVars builds the env vars needed by the maintenance Job/CronJob.
func maintenanceEnvVars() []corev1.EnvVar {
	keys := []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"K8S_NAMESPACE", "DOCKER_REGISTRY", "DOCKER_IMAGE", "SIDECAR_IMAGE",
	}
	var envs []corev1.EnvVar
	for _, k := range keys {
		envs = append(envs, corev1.EnvVar{Name: k, Value: os.Getenv(k)})
	}
	return envs
}

// TriggerMaintenance creates a one-off Job to run maintenance tasks.
func (s *Server) TriggerMaintenance(ctx context.Context, _ *sacv1.Empty) (*sacv1.SuccessMessage, error) {
	if s.maintenanceImage == "" {
		return nil, grpcerr.Internal("maintenance image not configured", nil)
	}
	if err := s.containerManager.CreateOneOffJob(ctx, "maintenance", s.maintenanceImage, maintenanceEnvVars()); err != nil {
		return nil, grpcerr.Internal("Failed to trigger maintenance", err)
	}
	return &sacv1.SuccessMessage{Message: "Maintenance job triggered"}, nil
}

// intervalToCron converts a Go duration string (e.g. "10m", "1h") to a cron expression.
func intervalToCron(interval string) string {
	dur, err := time.ParseDuration(interval)
	if err != nil || dur < time.Minute {
		return "*/10 * * * *" // default 10m
	}

	minutes := int(dur.Minutes())
	if minutes <= 0 {
		minutes = 10
	}

	if minutes < 60 {
		return fmt.Sprintf("*/%d * * * *", minutes)
	}

	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("0 */%d * * *", hours)
	}

	return "0 0 * * *" // daily
}

// ReconcileMaintenanceCronJob reads the skill_sync_interval setting and ensures
// the CronJob schedule matches. Call on startup and after updating the setting.
func (s *Server) ReconcileMaintenanceCronJob(ctx context.Context) {
	if s.maintenanceImage == "" {
		log.Warn().Msg("maintenance: image not configured, skipping CronJob reconciliation")
		return
	}

	interval := "10m"
	var setting models.SystemSetting
	err := s.db.NewSelect().Model(&setting).Where("key = ?", "skill_sync_interval").Scan(ctx)
	if err == nil {
		val := strings.Trim(string(setting.Value), "\"")
		if val != "" {
			interval = val
		}
	}

	schedule := intervalToCron(interval)
	envVars := maintenanceEnvVars()

	if err := s.containerManager.EnsureCronJob(ctx, "maintenance", schedule, s.maintenanceImage, envVars); err != nil {
		log.Error().Err(err).Msg("maintenance: failed to reconcile CronJob")
	}
}
