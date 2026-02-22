package session

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/workspace"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	sacv1.UnimplementedSessionServiceServer
	db               *bun.DB
	containerManager *container.Manager
	syncService      *skill.SyncService
	settingsService  *admin.SettingsService
	workspaceSyncSvc *workspace.SyncService
}

func NewServer(db *bun.DB, containerManager *container.Manager, syncService *skill.SyncService, settingsService *admin.SettingsService, workspaceSyncSvc *workspace.SyncService) *Server {
	return &Server{
		db:               db,
		containerManager: containerManager,
		syncService:      syncService,
		settingsService:  settingsService,
		workspaceSyncSvc: workspaceSyncSvc,
	}
}

func (s *Server) CreateSession(ctx context.Context, req *sacv1.CreateSessionRequest) (*sacv1.CreateSessionResponse, error) {
	userID := ctxkeys.UserID(ctx)
	userIDStr := fmt.Sprintf("%d", userID)

	if req.AgentId <= 0 {
		return nil, grpcerr.BadRequest("agent_id is required")
	}

	// GetOrCreate: try to reuse an existing session for this user+agent
	var existing models.Session
	err := s.db.NewSelect().
		Model(&existing).
		Where("user_id = ?", userID).
		Where("agent_id = ?", req.AgentId).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusIdle,
		})).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)

	if err == nil {
		podIP, podErr := s.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentId)
		if podErr == nil && podIP != "" {
			now := time.Now()
			_, _ = s.db.NewUpdate().
				Model((*models.Session)(nil)).
				Set("pod_ip = ?", podIP).
				Set("last_active = ?", now).
				Set("status = ?", models.SessionStatusRunning).
				Set("updated_at = ?", now).
				Where("id = ?", existing.ID).
				Exec(ctx)

			log.Printf("Reusing existing session: sessionID=%s, agentID=%d, podIP=%s", existing.SessionID, req.AgentId, podIP)

			return &sacv1.CreateSessionResponse{
				SessionId: existing.SessionID,
				Status:    string(models.SessionStatusRunning),
				PodName:   existing.PodName,
				CreatedAt: timestamppb.New(existing.CreatedAt),
			}, nil
		}

		log.Printf("Existing session %s has unhealthy pod, marking as deleted", existing.SessionID)
		_, _ = s.db.NewUpdate().
			Model((*models.Session)(nil)).
			Set("status = ?", models.SessionStatusDeleted).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", existing.ID).
			Exec(ctx)
	}

	// Create new session
	sessionID := uuid.New().String()
	log.Printf("Creating session: userID=%s, sessionID=%s, agentID=%d", userIDStr, sessionID, req.AgentId)

	// Close sessions for OTHER agents
	_, err = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("user_id = ?", userID).
		Where("agent_id != ?", req.AgentId).
		Where("status IN (?)", bun.In([]models.SessionStatus{
			models.SessionStatusRunning,
			models.SessionStatusCreating,
			models.SessionStatusIdle,
		})).
		Exec(ctx)
	if err != nil {
		log.Printf("Warning: failed to auto-close other agent sessions: %v", err)
	}

	// Load agent configuration
	var agent models.Agent
	err = s.db.NewSelect().Model(&agent).
		Where("id = ?", req.AgentId).
		Where("created_by = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Agent not found", err)
	}

	// Check if StatefulSet exists
	isNewStatefulSet := false
	sts, err := s.containerManager.GetStatefulSet(ctx, userIDStr, req.AgentId)
	if err != nil {
		isNewStatefulSet = true
		log.Printf("StatefulSet not found, creating it...")

		limits := s.settingsService.GetResourceLimits(ctx, userID)
		rc := &container.ResourceConfig{
			CPURequest:    limits.CPURequest,
			CPULimit:      limits.CPULimit,
			MemoryRequest: limits.MemoryRequest,
			MemoryLimit:   limits.MemoryLimit,
		}
		if agent.CPURequest != nil {
			rc.CPURequest = *agent.CPURequest
		}
		if agent.CPULimit != nil {
			rc.CPULimit = *agent.CPULimit
		}
		if agent.MemoryRequest != nil {
			rc.MemoryRequest = *agent.MemoryRequest
		}
		if agent.MemoryLimit != nil {
			rc.MemoryLimit = *agent.MemoryLimit
		}

		dockerImage := s.settingsService.GetDockerImage(ctx)

		if err := s.containerManager.CreateStatefulSet(ctx, userIDStr, req.AgentId, agent.Config, rc, dockerImage); err != nil {
			return nil, grpcerr.Internal("Failed to create StatefulSet", err)
		}

		if err := s.containerManager.WaitForStatefulSetReady(ctx, userIDStr, req.AgentId, 60, 5*time.Second); err != nil {
			log.Printf("Warning: %v", err)
		}
	} else {
		log.Printf("Using existing StatefulSet: %s", sts.Name)
	}

	podIP, err := s.containerManager.GetStatefulSetPodIP(ctx, userIDStr, req.AgentId)
	if err != nil {
		return nil, grpcerr.Internal("Failed to get Pod IP, pod may not be ready", err)
	}

	if isNewStatefulSet {
		if err := s.syncService.SyncAllSkillsToAgent(ctx, userIDStr, req.AgentId); err != nil {
			log.Printf("Warning: failed to sync skills to agent %d: %v", req.AgentId, err)
		}
		s.writeClaudeMD(ctx, userIDStr, req.AgentId, agent.Instructions)
	} else {
		go func() {
			bgCtx := context.Background()
			if s.workspaceSyncSvc != nil {
				if err := s.workspaceSyncSvc.SyncWorkspaceToPod(bgCtx, userIDStr, req.AgentId); err != nil {
					log.Printf("Warning: background workspace sync failed: %v", err)
				}
			}
			if err := s.syncService.SyncAllSkillsToAgent(bgCtx, userIDStr, req.AgentId); err != nil {
				log.Printf("Warning: background skill sync failed: %v", err)
			}
			s.writeClaudeMD(bgCtx, userIDStr, req.AgentId, agent.Instructions)
			log.Printf("Background sync completed for user %s agent %d", userIDStr, req.AgentId)
		}()
	}

	stsName := fmt.Sprintf("claude-code-%s-%d", userIDStr, req.AgentId)
	session := &models.Session{
		UserID:     userID,
		AgentID:    req.AgentId,
		SessionID:  sessionID,
		PodName:    stsName,
		PodIP:      podIP,
		Status:     models.SessionStatusRunning,
		LastActive: time.Now(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = s.db.NewInsert().Model(session).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to save session", err)
	}

	return &sacv1.CreateSessionResponse{
		SessionId: sessionID,
		Status:    string(models.SessionStatusRunning),
		PodName:   stsName,
		CreatedAt: timestamppb.New(session.CreatedAt),
		IsNew:     isNewStatefulSet,
	}, nil
}

func (s *Server) GetSession(ctx context.Context, req *sacv1.GetSessionRequest) (*sacv1.Session, error) {
	userID := ctxkeys.UserID(ctx)

	var session models.Session
	err := s.db.NewSelect().
		Model(&session).
		Where("session_id = ?", req.SessionId).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Session not found", err)
	}

	return convert.SessionToProto(&session), nil
}

func (s *Server) ListSessions(ctx context.Context, _ *sacv1.Empty) (*sacv1.UserSessionListResponse, error) {
	userID := ctxkeys.UserID(ctx)

	var sessions []models.Session
	err := s.db.NewSelect().
		Model(&sessions).
		Where("user_id = ?", userID).
		Where("status != ?", models.SessionStatusDeleted).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to list sessions", err)
	}

	return &sacv1.UserSessionListResponse{Sessions: convert.SessionsToProto(sessions)}, nil
}

func (s *Server) DeleteSession(ctx context.Context, req *sacv1.GetSessionRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	var session models.Session
	err := s.db.NewSelect().
		Model(&session).
		Where("session_id = ?", req.SessionId).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("Session not found", err)
	}

	_, err = s.db.NewUpdate().
		Model((*models.Session)(nil)).
		Set("status = ?", models.SessionStatusDeleted).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", session.ID).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to delete session", err)
	}

	return &sacv1.SuccessMessage{Message: "Session deleted successfully"}, nil
}

func (s *Server) getGroupTemplates(ctx context.Context, userID int64) []string {
	var templates []string
	err := s.db.NewSelect().
		TableExpr("groups AS g").
		ColumnExpr("g.claude_md_template").
		Join("JOIN group_members AS gm ON gm.group_id = g.id").
		Where("gm.user_id = ?", userID).
		Where("g.claude_md_template != ''").
		OrderExpr("g.name ASC").
		Scan(ctx, &templates)
	if err != nil {
		log.Printf("Warning: failed to get group templates for user %d: %v", userID, err)
		return nil
	}
	return templates
}

func (s *Server) writeClaudeMD(ctx context.Context, userIDStr string, agentID int64, agentInstructions string) {
	sysInstructions := s.settingsService.GetAgentSystemInstructions(ctx)
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	groupTemplates := s.getGroupTemplates(ctx, userID)

	if sysInstructions == "" && len(groupTemplates) == 0 && agentInstructions == "" {
		return
	}

	var parts []string
	if sysInstructions != "" {
		parts = append(parts, sysInstructions)
	}
	parts = append(parts, groupTemplates...)
	if agentInstructions != "" {
		parts = append(parts, agentInstructions)
	}

	content := strings.Join(parts, "\n\n---\n\n")
	podName := fmt.Sprintf("claude-code-%s-%d-0", userIDStr, agentID)
	if err := s.containerManager.WriteFileInPod(ctx, podName, "/workspace/CLAUDE.md", content); err != nil {
		log.Printf("Warning: failed to write CLAUDE.md to pod %s: %v", podName, err)
	}
}
