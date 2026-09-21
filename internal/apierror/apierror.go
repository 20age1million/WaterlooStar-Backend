// Package apierror defines the single error envelope every failing request
// leaves the service through.
//
// It lives in its own package so both middleware and handlers can write it
// without an import cycle. The shape here must stay identical to the Error
// schema in api/openapi.yaml — the frontend has exactly one error contract to
// handle, and that is the point.
package apierror

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Code is a stable, machine-readable reason. Clients branch on these; the
// message is for humans and may be reworded freely.
type Code string

const (
	CodeBadRequest   Code = "bad_request"
	CodeValidation   Code = "validation_failed"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeInternal     Code = "internal_error"
	CodeUnavailable  Code = "service_unavailable"
)

// Envelope is the body of every error response.
type Envelope struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Details carries field-level problems for validation failures. Omitted otherwise.
	Details map[string]string `json:"details,omitempty"`
	// RequestID lets a user quote something specific when reporting a problem.
	RequestID string `json:"request_id,omitempty"`
}

// RequestIDKey is the gin context key holding the current request id.
const RequestIDKey = "request_id"

// Write aborts the request with the standard envelope.
func Write(c *gin.Context, status int, code Code, message string) {
	writeEnvelope(c, status, Envelope{Code: code, Message: message})
}

// WriteDetails aborts the request with field-level detail attached.
func WriteDetails(c *gin.Context, status int, code Code, message string, details map[string]string) {
	writeEnvelope(c, status, Envelope{Code: code, Message: message, Details: details})
}

func writeEnvelope(c *gin.Context, status int, env Envelope) {
	if id, ok := c.Get(RequestIDKey); ok {
		if s, ok := id.(string); ok {
			env.RequestID = s
		}
	}
	c.AbortWithStatusJSON(status, env)
}

// NotFound writes the standard 404. Used both by handlers and by the router's
// fallback, so an unknown path and an unknown record look the same to a client.
func NotFound(c *gin.Context, message string) {
	Write(c, http.StatusNotFound, CodeNotFound, message)
}

// Internal writes the standard 500. The underlying error is logged, never
// returned — a client has no use for it and it can leak schema details.
func Internal(c *gin.Context) {
	Write(c, http.StatusInternalServerError, CodeInternal, "Something went wrong on our end. Try again.")
}
