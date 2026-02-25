package main

import (
	"os"
	"os/signal"
	"syscall"

	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/websocket"
	"g.echo.tech/dev/sac/pkg/config"
	"g.echo.tech/dev/sac/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	logger.Init(cfg.LogLevel, cfg.LogFormat)

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize database")
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
	log.Info().Str("addr", addr).Msg("WebSocket Proxy starting")

	// Graceful shutdown
	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down WebSocket Proxy")
}
