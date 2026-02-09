package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/agent"
	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/session"
	"g.echo.tech/dev/sac/internal/skill"
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

	containerMgr, err := container.NewManager(cfg.KubeconfigPath, cfg.Namespace, cfg.DockerRegistry, cfg.DockerImage)
	if err != nil {
		log.Fatalf("Failed to create container manager: %v", err)
	}

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

		// Skill routes
		skillHandler := skill.NewHandler(database.DB, containerMgr)
		skillHandler.RegisterRoutes(protectedGroup)

		// Shared sync service for agent & session handlers
		syncService := skillHandler.GetSyncService()

		// Session routes
		sessionHandler := session.NewHandler(database.DB, containerMgr, syncService, settingsService)
		sessionHandler.RegisterRoutes(protectedGroup)

		// Agent routes
		agentHandler := agent.NewHandler(database.DB, containerMgr, syncService, settingsService)
		agentHandler.RegisterRoutes(protectedGroup)

		// Admin routes (requires admin role, checked inside RegisterRoutes)
		adminHandler := admin.NewHandler(database.DB, containerMgr)
		adminHandler.RegisterRoutes(protectedGroup)
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
