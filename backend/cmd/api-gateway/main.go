package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"g.echo.tech/dev/sac/internal/agent"
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

	// Mock auth middleware (for development)
	router.Use(func(c *gin.Context) {
		// In production, validate JWT token here
		// For now, use a mock user ID
		c.Set("userID", int64(1))
		c.Next()
	})

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	// Create shared container manager
	containerMgr, err := container.NewManager(cfg.KubeconfigPath, cfg.Namespace, cfg.DockerRegistry, cfg.DockerImage)
	if err != nil {
		log.Fatalf("Failed to create container manager: %v", err)
	}

	// Register API routes
	apiGroup := router.Group("/api")
	{
		// Skill routes (creates its own SyncService internally)
		skillHandler := skill.NewHandler(database.DB, containerMgr)
		skillHandler.RegisterRoutes(apiGroup)

		// Shared sync service for agent & session handlers
		syncService := skillHandler.GetSyncService()

		// Session routes
		sessionHandler := session.NewHandler(database.DB, containerMgr, syncService)
		sessionHandler.RegisterRoutes(apiGroup)

		// Agent routes
		agentHandler := agent.NewHandler(database.DB, containerMgr, syncService)
		agentHandler.RegisterRoutes(apiGroup)
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
