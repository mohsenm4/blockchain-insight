package api

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// SlogLogger returns a gin middleware that logs one structured line per
// request via slog.Default. It picks the level from the response status so
// noisy 200s stay at Info while 5xx surface at Error.
func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
		}

		if err := c.Errors.String(); err != "" {
			attrs = append(attrs, slog.String("error", err))
		}

		switch {
		case status >= 500:
			slog.Error("http request", attrs...)
		case status >= 400:
			slog.Warn("http request", attrs...)
		default:
			slog.Info("http request", attrs...)
		}
	}
}
