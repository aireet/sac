package admin

import (
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
)

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role.(string) != "admin" {
			response.Forbidden(c, "Admin access required")
			c.Abort()
			return
		}
		c.Next()
	}
}
