package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/echotech/sac/internal/agent"
	"github.com/echotech/sac/internal/database"
	"github.com/echotech/sac/internal/session"
	"github.com/echotech/sac/internal/skill"
	"github.com/echotech/sac/pkg/config"
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

	// Register API routes
	apiGroup := router.Group("/api")
	{
		// Skill routes
		skillHandler := skill.NewHandler(database.DB)
		skillHandler.RegisterRoutes(apiGroup)

		// Session routes
		sessionHandler, err := session.NewHandler(database.DB, cfg)
		if err != nil {
			log.Fatalf("Failed to create session handler: %v", err)
		}
		sessionHandler.RegisterRoutes(apiGroup)

		// Agent routes
		agentRoutes := apiGroup.Group("/agents")
		{
			agentRoutes.GET("", agent.GetAgents)
			agentRoutes.GET("/:id", agent.GetAgent)
			agentRoutes.POST("", agent.CreateAgent)
			agentRoutes.PUT("/:id", agent.UpdateAgent)
			agentRoutes.DELETE("/:id", agent.DeleteAgent)
			agentRoutes.POST("/:id/skills", agent.InstallSkill)
			agentRoutes.DELETE("/:id/skills/:skillId", agent.UninstallSkill)
		}
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
