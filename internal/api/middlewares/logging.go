package middlewares

import (
    "time"

    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

func Logger(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        method := c.Request.Method

        c.Next()

        latency := time.Since(start)
        statusCode := c.Writer.Status()

        logger.Info("request",
            zap.String("method", method),
            zap.String("path", path),
            zap.Int("status", statusCode),
            zap.Duration("latency", latency),
            zap.String("client_ip", c.ClientIP()),
        )
    }
}
