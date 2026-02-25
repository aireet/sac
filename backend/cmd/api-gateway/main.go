package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/admin"
	"g.echo.tech/dev/sac/internal/agent"
	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/container"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"g.echo.tech/dev/sac/internal/database"
	"g.echo.tech/dev/sac/internal/group"
	"g.echo.tech/dev/sac/internal/history"
	sacredis "g.echo.tech/dev/sac/internal/redis"
	"g.echo.tech/dev/sac/internal/session"
	"g.echo.tech/dev/sac/internal/skill"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/internal/workspace"
	"g.echo.tech/dev/sac/pkg/config"
	"g.echo.tech/dev/sac/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
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

	// Create shared services
	jwtService := auth.NewJWTService(cfg.JWTSecret)
	settingsService := admin.NewSettingsService(database.DB)

	containerMgr, err := container.NewManager(cfg.KubeconfigPath, cfg.Namespace, cfg.DockerRegistry, cfg.DockerImage, cfg.SidecarImage)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create container manager")
	}

	storageProvider := storage.NewStorageProvider(database.DB)

	// Initialize Redis (optional)
	var outputHub *workspace.OutputHub
	var syncHub *skill.SyncHub
	if cfg.RedisURL == "" {
		log.Warn().Msg("REDIS_URL not set, output watch disabled")
	} else if err := sacredis.Initialize(cfg.RedisURL); err != nil {
		log.Warn().Err(err).Msg("Redis not available, output watch disabled")
	} else {
		defer sacredis.Close()
		outputHub = workspace.NewOutputHub(sacredis.Client)
		syncHub = skill.NewSyncHub(sacredis.Client)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go outputHub.Start(ctx)
		go syncHub.Start(ctx)
	}

	// ---- gRPC Server (in-process, no network listener) ----
	grpcServer := grpc.NewServer()

	// Create skill handler to get SyncService (shared dependency)
	skillHandler := skill.NewHandler(database.DB, containerMgr, storageProvider)
	syncService := skillHandler.GetSyncService()

	// Wire sync progress publisher (nil-safe: if syncHub is nil, events are dropped)
	if syncHub != nil {
		syncService.SetPublisher(syncHub)
	}

	// Register all gRPC service implementations
	authServer := auth.NewServer(database.DB, jwtService, settingsService)
	sacv1.RegisterAuthServiceServer(grpcServer, authServer)

	skillServer := skill.NewServer(database.DB, syncService)
	sacv1.RegisterSkillServiceServer(grpcServer, skillServer)

	groupServer := group.NewServer(database.DB)
	sacv1.RegisterGroupServiceServer(grpcServer, groupServer)
	sacv1.RegisterAdminGroupServiceServer(grpcServer, groupServer)

	historyServer := history.NewServer(database.DB)
	sacv1.RegisterHistoryServiceServer(grpcServer, historyServer)

	agentServer := agent.NewServer(database.DB, containerMgr, syncService, settingsService, syncHub)
	sacv1.RegisterAgentServiceServer(grpcServer, agentServer)

	sessionServer := session.NewServer(database.DB, containerMgr, syncService, settingsService, storageProvider)
	sacv1.RegisterSessionServiceServer(grpcServer, sessionServer)

	adminServer := admin.NewServer2(database.DB, containerMgr, fmt.Sprintf("%s/%s", cfg.DockerRegistry, cfg.DockerImage))
	sacv1.RegisterAdminServiceServer(grpcServer, adminServer)

	workspaceServer := workspace.NewWorkspaceServer(database.DB, storageProvider, outputHub)
	sacv1.RegisterWorkspaceServiceServer(grpcServer, workspaceServer)

	// ---- gRPC-Gateway Mux (in-process calls) ----
	ctx := context.Background()
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   true,
				EmitUnpopulated: false,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)

	// Register all gateway handlers (in-process, no network)
	must(sacv1.RegisterAuthServiceHandlerServer(ctx, gwMux, authServer))
	must(sacv1.RegisterSkillServiceHandlerServer(ctx, gwMux, skillServer))
	must(sacv1.RegisterGroupServiceHandlerServer(ctx, gwMux, groupServer))
	must(sacv1.RegisterAdminGroupServiceHandlerServer(ctx, gwMux, groupServer))
	must(sacv1.RegisterHistoryServiceHandlerServer(ctx, gwMux, historyServer))
	must(sacv1.RegisterAgentServiceHandlerServer(ctx, gwMux, agentServer))
	must(sacv1.RegisterSessionServiceHandlerServer(ctx, gwMux, sessionServer))
	must(sacv1.RegisterAdminServiceHandlerServer(ctx, gwMux, adminServer))
	must(sacv1.RegisterWorkspaceServiceHandlerServer(ctx, gwMux, workspaceServer))

	// ---- Gin Router (special endpoints only) ----
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

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Workspace handler for output file download/WS/SSE endpoints
	workspaceHandler := workspace.NewHandler(database.DB, storageProvider, outputHub, jwtService)

	// Internal routes (no JWT, pod-internal calls) — only multipart upload
	internalGroup := router.Group("/api/internal")
	internalGroup.POST("/output/upload", workspaceHandler.RequireOSS(), workspaceHandler.InternalOutputUpload)

	// History internal route (pod-internal, no JWT)
	historyHandler := history.NewHandler(database.DB)
	historyHandler.RegisterInternalRoutes(internalGroup)

	// Public routes (no auth) — WS and shared file download
	router.GET("/api/workspace/output/watch", workspaceHandler.WatchOutput)
	router.GET("/api/skill-sync/watch", skill.WatchSync(syncHub, jwtService))
	router.GET("/api/s/:code/raw", workspaceHandler.RequireOSS(), workspaceHandler.DownloadSharedFile)

	// Protected file routes (JWT auth + multipart/streaming)
	protected := router.Group("/api")
	protected.Use(auth.AuthMiddleware(jwtService))
	{
		ws := protected.Group("/workspace")
		ws.GET("/output/files/download", workspaceHandler.RequireOSS(), workspaceHandler.DownloadOutputFile)

		// Skill file management (multipart upload, not suitable for gRPC-gateway)
		skillHandler.RegisterFileRoutes(protected)

		// CSV exports (streaming response, not suitable for gRPC-gateway)
		protected.GET("/conversations/export", historyHandler.ExportConversations)

		adminGroup := protected.Group("/admin")
		adminGroup.Use(admin.AdminMiddleware())
		adminHandler := admin.NewHandler(database.DB, containerMgr)
		adminGroup.GET("/conversations/export", adminHandler.ExportConversations)
	}

	// Fallback: all unmatched routes go to gRPC-gateway with JWT auth injected.
	router.NoRoute(gatewayAuthMiddleware(jwtService, gwMux))

	// Reconcile maintenance CronJob on startup
	go adminServer.ReconcileMaintenanceCronJob(context.Background())

	// Start server
	addr := "0.0.0.0:" + cfg.APIGatewayPort
	log.Info().Str("addr", addr).Msg("API Gateway starting (hybrid Gin + gRPC-gateway)")

	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down API Gateway")
	grpcServer.GracefulStop()
}

func must(err error) {
	if err != nil {
		log.Fatal().Err(err).Msg("failed to register gateway handler")
	}
}

// publicPaths are routes that don't require JWT authentication.
var publicPaths = map[string]bool{
	"/api/auth/register":          true,
	"/api/auth/login":             true,
	"/api/auth/registration-mode": true,
	"/api/s/":                     false, // prefix match below
}

// gatewayResponseWriter wraps http.ResponseWriter to override the status code
// that Gin's NoRoute pre-sets to 404. The gRPC-gateway will call WriteHeader
// with the correct status, and we forward that instead of Gin's 404.
type gatewayResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *gatewayResponseWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *gatewayResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// gatewayAuthMiddleware wraps the gRPC-gateway mux with JWT authentication.
func gatewayAuthMiddleware(jwtService *auth.JWTService, gwMux http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Wrap the writer so gRPC-gateway controls the status code, not Gin's NoRoute 404.
		gw := &gatewayResponseWriter{ResponseWriter: c.Writer}

		// Public routes — no auth needed
		if publicPaths[path] || strings.HasPrefix(path, "/api/s/") ||
			strings.HasPrefix(path, "/api/internal/") {
			gwMux.ServeHTTP(gw, c.Request)
			return
		}

		// All other routes require a valid JWT
		authHeader := c.GetHeader("Authorization")
		tokenStr, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || tokenStr == "" {
			c.JSON(401, gin.H{"code": 16, "message": "authorization header required"})
			c.Abort()
			return
		}

		claims, err := jwtService.ValidateToken(tokenStr)
		if err != nil {
			c.JSON(401, gin.H{"code": 16, "message": "invalid or expired token"})
			c.Abort()
			return
		}

		// Admin routes require admin role
		if strings.HasPrefix(path, "/api/admin/") && claims.Role != "admin" {
			c.JSON(403, gin.H{"code": 7, "message": "admin access required"})
			c.Abort()
			return
		}

		// Inject user claims into context for gRPC-gateway server methods
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, ctxkeys.UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, ctxkeys.UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, ctxkeys.RoleKey, claims.Role)
		c.Request = c.Request.WithContext(ctx)

		gwMux.ServeHTTP(gw, c.Request)
	}
}
