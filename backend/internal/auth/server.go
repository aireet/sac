package auth

import (
	"context"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/grpcerr"
	"g.echo.tech/dev/sac/internal/models"
	"github.com/uptrace/bun"
)

type Server struct {
	sacv1.UnimplementedAuthServiceServer
	db              *bun.DB
	jwtService      *JWTService
	settingsService *admin.SettingsService
}

func 
NewServer(db *bun.DB, jwtService *JWTService, settingsService *admin.SettingsService) *Server {
	return &Server{db: db, jwtService: jwtService, settingsService: settingsService}
}

func (s *Server) Register(ctx context.Context, req *sacv1.RegisterRequest) (*sacv1.AuthResponse, error) {
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, grpcerr.BadRequest("username, email, and password are required")
	}
	if len(req.Password) < 6 {
		return nil, grpcerr.BadRequest("password must be at least 6 characters")
	}

	mode, _ := s.settingsService.GetSetting(ctx, "registration_mode")
	if mode == "invite" {
		return nil, grpcerr.Forbidden("Registration is disabled")
	}

	exists, err := s.db.NewSelect().Model((*models.User)(nil)).
		Where("username = ? OR email = ?", req.Username, req.Email).
		Exists(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to check user existence", err)
	}
	if exists {
		return nil, grpcerr.Conflict("Username or email already exists")
	}

	hashedPassword, err := HashPassword(req.Password)
	if err != nil {
		return nil, grpcerr.Internal("Failed to hash password", err)
	}

	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hashedPassword,
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err = s.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to create user", err)
	}

	token, err := s.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, grpcerr.Internal("Failed to generate token", err)
	}

	return &sacv1.AuthResponse{Token: token, User: convert.UserToProto(user)}, nil
}

func (s *Server) Login(ctx context.Context, req *sacv1.LoginRequest) (*sacv1.AuthResponse, error) {
	if req.Username == "" || req.Password == "" {
		return nil, grpcerr.BadRequest("username and password are required")
	}

	var user models.User
	err := s.db.NewSelect().Model(&user).
		Where("username = ? OR email = ?", req.Username, req.Username).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Unauthorized("Invalid credentials")
	}

	if !CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, grpcerr.Unauthorized("Invalid credentials")
	}

	token, err := s.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, grpcerr.Internal("Failed to generate token", err)
	}

	return &sacv1.AuthResponse{Token: token, User: convert.UserToProto(&user)}, nil
}

func (s *Server) GetRegistrationMode(ctx context.Context, _ *sacv1.Empty) (*sacv1.RegistrationModeResponse, error) {
	mode, _ := s.settingsService.GetSetting(ctx, "registration_mode")
	if mode == "" {
		mode = "invite"
	}
	return &sacv1.RegistrationModeResponse{Mode: mode}, nil
}

func (s *Server) GetCurrentUser(ctx context.Context, _ *sacv1.Empty) (*sacv1.User, error) {
	userID := ctxkeys.UserID(ctx)

	var user models.User
	err := s.db.NewSelect().Model(&user).Where("id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("User not found", err)
	}

	return convert.UserToProto(&user), nil
}

func (s *Server) ChangePassword(ctx context.Context, req *sacv1.ChangePasswordRequest) (*sacv1.SuccessMessage, error) {
	userID := ctxkeys.UserID(ctx)

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return nil, grpcerr.BadRequest("current_password and new_password are required")
	}
	if len(req.NewPassword) < 6 {
		return nil, grpcerr.BadRequest("new password must be at least 6 characters")
	}

	var user models.User
	err := s.db.NewSelect().Model(&user).Where("id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, grpcerr.NotFound("User not found", err)
	}

	if !CheckPasswordHash(req.CurrentPassword, user.PasswordHash) {
		return nil, grpcerr.BadRequest("current password is incorrect")
	}

	if req.CurrentPassword == req.NewPassword {
		return nil, grpcerr.BadRequest("new password must be different from current password")
	}

	hashedPassword, err := HashPassword(req.NewPassword)
	if err != nil {
		return nil, grpcerr.Internal("Failed to hash password", err)
	}

	_, err = s.db.NewUpdate().Model((*models.User)(nil)).
		Set("password_hash = ?", hashedPassword).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to update password", err)
	}

	return &sacv1.SuccessMessage{Message: "Password changed successfully"}, nil
}

func (s *Server) SearchUsers(ctx context.Context, req *sacv1.SearchUsersRequest) (*sacv1.UserBriefListResponse, error) {
	if len(req.Q) < 1 {
		return nil, grpcerr.BadRequest("Search query too short")
	}

	var users []models.User
	err := s.db.NewSelect().Model(&users).
		Where("username ILIKE ? OR display_name ILIKE ?", "%"+req.Q+"%", "%"+req.Q+"%").
		Column("id", "username", "display_name").
		Limit(20).
		Scan(ctx)
	if err != nil {
		return nil, grpcerr.Internal("Failed to search users", err)
	}

	result := make([]*sacv1.UserBrief, len(users))
	for i := range users {
		result[i] = convert.UserBriefToProto(&users[i])
	}
	return &sacv1.UserBriefListResponse{Users: result}, nil
}
