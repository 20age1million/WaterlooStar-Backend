// Package middleware holds the cross-cutting gin handlers every route passes
// through: request identity, structured logging, panic recovery and CORS.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/20age1million/waterloostar-api/internal/apierror"
)

// RequestIDHeader is echoed back on every response so a client can quote it.
const RequestIDHeader = "X-Request-ID"

// RequestID attaches an id to each request, reusing an inbound one when a proxy
// or the frontend has already assigned it.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(apierror.RequestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// Logger emits one structured line per completed request. It logs at error level
// for 5xx and warn for 4xx so a noisy client is visible without raising alerts.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("took", time.Since(start)),
			slog.String("request_id", c.GetString(apierror.RequestIDKey)),
		}
		if query != "" {
			attrs = append(attrs, slog.String("query", query))
		}
		if err := c.Errors.Last(); err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}

		switch {
		case status >= http.StatusInternalServerError:
			log.Error("request failed", attrs...)
		case status >= http.StatusBadRequest:
			log.Warn("request rejected", attrs...)
		default:
			log.Info("request", attrs...)
		}
	}
}

// Recovery converts a panic into the standard 500 envelope and logs the cause
// with its stack. gin's own recovery writes a bare status, which would be the
// one response in the service not matching the documented error contract.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("panic recovered",
					slog.Any("panic", recovered),
					slog.String("path", c.Request.URL.Path),
					slog.String("request_id", c.GetString(apierror.RequestIDKey)),
					slog.String("stack", stack()),
				)
				apierror.Internal(c)
			}
		}()
		c.Next()
	}
}

// CORS allows the configured frontend origin to make credentialed requests.
// Credentials are required because the session JWT travels as an httpOnly
// cookie, which in turn means the origin must be echoed exactly — a wildcard
// is not permitted alongside credentials.
func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.GetHeader("Origin")
		if requestOrigin != "" && requestOrigin == origin {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, "+RequestIDHeader+", X-CSRF-Token")
			h.Set("Access-Control-Expose-Headers", RequestIDHeader)
			h.Set("Access-Control-Max-Age", "600")
			h.Add("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
