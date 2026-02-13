package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/agent"
	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/group"
	"g.echo.tech/dev/sac/internal/history"
	sacredis "g.echo.tech/dev/sac/internal/redis"
	"g.echo.tech/dev/sac/internal/session"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/internal/workspace"
	"g.echo.tech/dev/sac/pkg/config"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Create Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check (public)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	// Create shared services
	jwtService := auth.NewJWTService(cfg.JWTSecret)
	settingsService := admin.NewSettingsService(database.DB)

	containerMgr, err := container.NewManager(cfg.KubeconfigPath, cfg.Namespace, cfg.DockerRegistry, cfg.DockerImage, cfg.SidecarImage)
	if err != nil {
		log.Fatalf("Failed to create container manager: %v", err)
	}

	// Storage provider reads config from system_settings (admin-managed)
	storageProvider := storage.NewStorageProvider(database.DB)
	workspaceSyncSvc := workspace.NewSyncService(database.DB, storageProvider, containerMgr)

	// Initialize Redis (optional — SSE output watch degrades gracefully)
	var outputHub *workspace.OutputHub
	if cfg.RedisURL == "" {
		log.Printf("Warning: REDIS_URL not set, output SSE disabled")
	} else if err := sacredis.Initialize(cfg.RedisURL); err != nil {
		log.Printf("Warning: Redis not available, output SSE disabled: %v", err)
	} else {
		defer sacredis.Close()
		outputHub = workspace.NewOutputHub(sacredis.Client)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go outputHub.Start(ctx)
	}

	// Shared history handler
	historyHandler := history.NewHandler(database.DB)

	// Workspace handler (needed for both internal and protected routes)
	workspaceHandler := workspace.NewHandler(database.DB, storageProvider, workspaceSyncSvc, outputHub)

	// Internal API routes (no JWT, Pod-internal calls)
	internalGroup := router.Group("/api/internal")
	historyHandler.RegisterInternalRoutes(internalGroup)
	workspaceHandler.RegisterInternalRoutes(internalGroup)

	// Public routes (no auth required)
	publicGroup := router.Group("/api")
	authHandler := auth.NewHandler(database.DB, jwtService)
	authHandler.RegisterRoutes(publicGroup, nil) // register public routes only

	// Protected routes (JWT auth required)
	protectedGroup := router.Group("/api")
	protectedGroup.Use(auth.AuthMiddleware(jwtService))
	{
		// Register auth /me route
		authHandler.RegisterRoutes(nil, protectedGroup)

		// Conversation history routes
		historyHandler.RegisterRoutes(protectedGroup)

		// Skill routes
		skillHandler := skill.NewHandler(database.DB, containerMgr)
		skillHandler.RegisterRoutes(protectedGroup)

		// Shared sync service for agent & session handlers
		syncService := skillHandler.GetSyncService()

		// Session routes
		sessionHandler := session.NewHandler(database.DB, containerMgr, syncService, settingsService, workspaceSyncSvc)
		sessionHandler.RegisterRoutes(protectedGroup)

		// Agent routes
		agentHandler := agent.NewHandler(database.DB, containerMgr, syncService, settingsService)
		agentHandler.RegisterRoutes(protectedGroup)

		// Workspace routes (always registered; requireOSS middleware returns 503 if not configured)
		workspaceHandler.RegisterRoutes(protectedGroup)

		// Group routes (read-only for authenticated users)
		groupHandler := group.NewHandler(database.DB)
		groupHandler.RegisterRoutes(protectedGroup)

		// Admin routes (requires admin role)
		adminGroup := protectedGroup.Group("/admin")
		adminGroup.Use(admin.AdminMiddleware())

		adminHandler := admin.NewHandler(database.DB, containerMgr)
		adminHandler.RegisterRoutes(adminGroup)
		groupHandler.RegisterAdminRoutes(adminGroup)
	}

	// Start server (listen on all interfaces for remote debugging)
	addr := "0.0.0.0:" + cfg.APIGatewayPort
	log.Printf("API Gateway starting on %s", addr)

	// Graceful shutdown
	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down API Gateway...")
}
