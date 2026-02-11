package auth

import (
	"context"
	"time"

	"g.echo.tech/dev/sac/internal/models"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

type Handler struct {
	db         *bun.DB
	jwtService *JWTService
}

func NewHandler(db *bun.DB, jwtService *JWTService) *Handler {
	return &Handler{db: db, jwtService: jwtService}
}

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
		DisplayName string `json:"display_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return
	}

	ctx := context.Background()

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

	response.Created(c, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body", err)
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

	response.OK(c, gin.H{
		"token": token,
		"user":  user,
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

	response.OK(c, user)
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

	response.OK(c, users)
}

func (h *Handler) RegisterRoutes(public *gin.RouterGroup, protected *gin.RouterGroup) {
	if public != nil {
		public.POST("/auth/register", h.Register)
		public.POST("/auth/login", h.Login)
	}
	if protected != nil {
		protected.GET("/auth/me", h.GetCurrentUser)
		protected.GET("/users/search", h.SearchUsers)
	}
}
