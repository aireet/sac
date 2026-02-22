package ctxkeys

import "context"

type contextKey string

const (
	UserIDKey   contextKey = "userID"
	UsernameKey contextKey = "username"
	RoleKey     contextKey = "role"
)

func UserID(ctx context.Context) int64 {
	v, _ := ctx.Value(UserIDKey).(int64)
	return v
}

func Username(ctx context.Context) string {
	v, _ := ctx.Value(UsernameKey).(string)
	return v
}

func Role(ctx context.Context) string {
	v, _ := ctx.Value(RoleKey).(string)
	return v
}
