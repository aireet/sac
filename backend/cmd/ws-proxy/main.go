package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/websocket"
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

	// Create JWT service for WebSocket auth
	jwtService := auth.NewJWTService(cfg.JWTSecret)

	// Create WebSocket proxy handler
	proxyHandler := websocket.NewProxyHandler(database.DB, jwtService)

	// Register routes
	router.GET("/health", proxyHandler.HealthCheck)
	router.GET("/ws/:sessionId", proxyHandler.HandleWebSocket)

	// Start server (listen on all interfaces for remote debugging)
	addr := "0.0.0.0:" + cfg.WSProxyPort
	log.Printf("WebSocket Proxy starting on %s", addr)

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

	log.Println("Shutting down WebSocket Proxy...")
}
