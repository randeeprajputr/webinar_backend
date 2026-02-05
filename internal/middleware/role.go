package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/aura-webinar/backend/pkg/response"
)

// RequireRole returns a middleware that allows only the given roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{})
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		roleVal, ok := c.Get(ContextUserRole)
		if !ok {
			response.Unauthorized(c, "missing user context")
			c.Abort()
			return
		}
		role, _ := roleVal.(string)
		if _, ok := allowed[role]; !ok {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}
		c.Next()
	}
}
