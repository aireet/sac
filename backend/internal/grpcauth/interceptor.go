package grpcauth

import (
	"context"
	"strings"

	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/ctxkeys"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// publicMethods do not require JWT authentication.
var publicMethods = map[string]bool{
	"/sac.v1.AuthService/Register":                  true,
	"/sac.v1.AuthService/Login":                     true,
	"/sac.v1.AuthService/GetRegistrationMode":       true,
	"/sac.v1.HistoryService/ReceiveEvents":          true,
	"/sac.v1.WorkspaceService/InternalOutputDelete": true,
	"/sac.v1.WorkspaceService/GetSharedFileMeta":    true,
}

// adminMethods require role=admin.
var adminMethods = map[string]bool{
	"/sac.v1.AdminService/GetSettings":              true,
	"/sac.v1.AdminService/UpdateSetting":            true,
	"/sac.v1.AdminService/GetUsers":                 true,
	"/sac.v1.AdminService/UpdateUserRole":           true,
	"/sac.v1.AdminService/GetUserSettings":          true,
	"/sac.v1.AdminService/SetUserSetting":           true,
	"/sac.v1.AdminService/DeleteUserSetting":        true,
	"/sac.v1.AdminService/GetUserAgents":            true,
	"/sac.v1.AdminService/DeleteUserAgent":          true,
	"/sac.v1.AdminService/RestartUserAgent":         true,
	"/sac.v1.AdminService/UpdateAgentResources":     true,
	"/sac.v1.AdminService/UpdateAgentImage":         true,
	"/sac.v1.AdminService/BatchUpdateImage":         true,
	"/sac.v1.AdminService/ResetUserPassword":        true,
	"/sac.v1.AdminService/GetConversations":         true,
	"/sac.v1.AdminGroupService/ListAllGroups":       true,
	"/sac.v1.AdminGroupService/CreateGroup":         true,
	"/sac.v1.AdminGroupService/UpdateGroup":         true,
	"/sac.v1.AdminGroupService/DeleteGroup":         true,
	"/sac.v1.AdminGroupService/ListMembersAdmin":    true,
	"/sac.v1.AdminGroupService/AddMember":           true,
	"/sac.v1.AdminGroupService/RemoveMember":        true,
	"/sac.v1.AdminGroupService/UpdateMemberRole":    true,
	"/sac.v1.AdminGroupService/AdminUpdateTemplate": true,
}

// AuthUnaryInterceptor returns a gRPC unary interceptor that validates JWT tokens
// from the "authorization" metadata header and injects user info into the context.
func AuthUnaryInterceptor(jwtService *auth.JWTService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Public methods skip auth
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// Extract authorization from gRPC metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			// Also check grpcgateway-authorization (grpc-gateway forwards it)
			authHeaders = md.Get("grpcgateway-authorization")
		}
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization header required")
		}

		tokenString := strings.TrimPrefix(authHeaders[0], "Bearer ")
		if tokenString == authHeaders[0] {
			return nil, status.Error(codes.Unauthenticated, "Bearer token required")
		}

		claims, err := jwtService.ValidateToken(tokenString)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		// Admin check
		if adminMethods[info.FullMethod] && claims.Role != "admin" {
			return nil, status.Error(codes.PermissionDenied, "admin access required")
		}

		// Inject user info into context
		ctx = context.WithValue(ctx, ctxkeys.UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, ctxkeys.UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, ctxkeys.RoleKey, claims.Role)

		return handler(ctx, req)
	}
}
