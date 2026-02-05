package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/pkg/response"
)

const (
	// ContextUserID is the key for user ID in gin context.
	ContextUserID = "user_id"
	// ContextUserRole is the key for user role in gin context.
	ContextUserRole = "user_role"
	// ContextUserEmail is the key for user email in gin context.
	ContextUserEmail = "user_email"
)

// JWT returns a middleware that validates JWT and sets user claims in context.
func JWT(jwtService *auth.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "invalid authorization header")
			c.Abort()
			return
		}
		claims, err := jwtService.Validate(parts[1])
		if err != nil {
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextUserRole, claims.Role)
		c.Set(ContextUserEmail, claims.Email)
		c.Next()
	}
}
