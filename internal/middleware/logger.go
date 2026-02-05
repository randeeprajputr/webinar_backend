package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger returns a zap-based request logging middleware.
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := 0

		c.Next()

		statusCode = c.Writer.Status()
		latency := time.Since(start)

		logger.Info("request",
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("client_ip", clientIP),
		)
	}
}
