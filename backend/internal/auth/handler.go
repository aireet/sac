package auth

import (
	"context"
	"time"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/convert"
	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db              *bun.DB
	jwtService      *JWTService
	settingsService *admin.SettingsService
}

func NewHandler(db *bun.DB, jwtService *JWTService, settingsService *admin.SettingsService) *Handler {
	return &Handler{db: db, jwtService: jwtService, settingsService: settingsService}
}

func (h *Handler) Register(c *gin.Context) {
	req := &sacv1.RegisterRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Username == "" || req.Email == "" || req.Password == "" {
		response.BadRequest(c, "username, email, and password are required")
		return
	}
	if len(req.Password) < 6 {
		response.BadRequest(c, "password must be at least 6 characters")
		return
	}

	ctx := context.Background()

	// Check registration mode
	mode, _ := h.settingsService.GetSetting(ctx, "registration_mode")
	if mode == "invite" {
		response.Forbidden(c, "Registration is disabled")
		return
	}

	// Check if username or email already exists
	exists, err := h.db.NewSelect().Model((*models.User)(nil)).
		Where("username = ? OR email = ?", req.Username, req.Email).
		Exists(ctx)
	if err != nil {
		response.InternalError(c, "Failed to check user existence", err)
		return
	}
	if exists {
		response.Conflict(c, "Username or email already exists")
		return
	}

	hashedPassword, err := HashPassword(req.Password)
	if err != nil {
		response.InternalError(c, "Failed to hash password", err)
		return
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

	_, err = h.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to create user", err)
		return
	}

	token, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		response.InternalError(c, "Failed to generate token", err)
		return
	}

	protobind.Created(c, &sacv1.AuthResponse{
		Token: token,
		User:  convert.UserToProto(user),
	})
}

func (h *Handler) Login(c *gin.Context) {
	req := &sacv1.LoginRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequest(c, "username and password are required")
		return
	}

	ctx := context.Background()

	var user models.User
	err := h.db.NewSelect().Model(&user).
		Where("username = ? OR email = ?", req.Username, req.Username).
		Scan(ctx)
	if err != nil {
		response.Unauthorized(c, "Invalid credentials")
		return
	}

	if !CheckPasswordHash(req.Password, user.PasswordHash) {
		response.Unauthorized(c, "Invalid credentials")
		return
	}

	token, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		response.InternalError(c, "Failed to generate token", err)
		return
	}

	protobind.OK(c, &sacv1.AuthResponse{
		Token: token,
		User:  convert.UserToProto(&user),
	})
}

func (h *Handler) GetCurrentUser(c *gin.Context) {
	userID := c.GetInt64("userID")

	ctx := context.Background()
	var user models.User
	err := h.db.NewSelect().Model(&user).Where("id = ?", userID).Scan(ctx)
	if err != nil {
		response.NotFound(c, "User not found", err)
		return
	}

	protobind.OK(c, convert.UserToProto(&user))
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID := c.GetInt64("userID")

	req := &sacv1.ChangePasswordRequest{}
	if !protobind.Bind(c, req) {
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		response.BadRequest(c, "current_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 6 {
		response.BadRequest(c, "new password must be at least 6 characters")
		return
	}

	ctx := context.Background()

	var user models.User
	err := h.db.NewSelect().Model(&user).Where("id = ?", userID).Scan(ctx)
	if err != nil {
		response.NotFound(c, "User not found", err)
		return
	}

	if !CheckPasswordHash(req.CurrentPassword, user.PasswordHash) {
		response.BadRequest(c, "current password is incorrect")
		return
	}

	if req.CurrentPassword == req.NewPassword {
		response.BadRequest(c, "new password must be different from current password")
		return
	}

	hashedPassword, err := HashPassword(req.NewPassword)
	if err != nil {
		response.InternalError(c, "Failed to hash password", err)
		return
	}

	_, err = h.db.NewUpdate().Model((*models.User)(nil)).
		Set("password_hash = ?", hashedPassword).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", userID).
		Exec(ctx)
	if err != nil {
		response.InternalError(c, "Failed to update password", err)
		return
	}

	protobind.OK(c, &sacv1.SuccessMessage{Message: "Password changed successfully"})
}

func (h *Handler) GetRegistrationMode(c *gin.Context) {
	ctx := context.Background()
	mode, _ := h.settingsService.GetSetting(ctx, "registration_mode")
	if mode == "" {
		mode = "invite"
	}
	protobind.OK(c, &sacv1.RegistrationModeResponse{Mode: mode})
}

func (h *Handler) SearchUsers(c *gin.Context) {
	q := c.Query("q")
	if len(q) < 1 {
		response.BadRequest(c, "Search query too short", nil)
		return
	}

	ctx := context.Background()
	var users []models.User
	err := h.db.NewSelect().Model(&users).
		Where("username ILIKE ? OR display_name ILIKE ?", "%"+q+"%", "%"+q+"%").
		Column("id", "username", "display_name").
		Limit(20).
		Scan(ctx)
	if err != nil {
		response.InternalError(c, "Failed to search users", err)
		return
	}

	result := make([]*sacv1.UserBrief, len(users))
	for i := range users {
		result[i] = convert.UserBriefToProto(&users[i])
	}
	protobind.OK(c, &sacv1.UserBriefListResponse{Users: result})
}

func (h *Handler) RegisterRoutes(public *gin.RouterGroup, protected *gin.RouterGroup) {
	if public != nil {
		public.POST("/auth/register", h.Register)
		public.POST("/auth/login", h.Login)
		public.GET("/auth/registration-mode", h.GetRegistrationMode)
	}
	if protected != nil {
		protected.GET("/auth/me", h.GetCurrentUser)
		protected.PUT("/auth/password", h.ChangePassword)
		protected.GET("/users/search", h.SearchUsers)
	}
}
