// Package httpapi builds the gin router and implements the handlers behind the
// interface generated from api/openapi.yaml.
package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/email"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
	"github.com/20age1million/WaterlooStar-Backend/internal/middleware"
)

// Server implements the generated StrictServerInterface. It holds everything a
// handler needs; handlers themselves stay free of global state.
//
// queries is the generated Querier interface rather than the concrete *Queries
// so handler tests can substitute a stub and exercise failure paths — an
// unreachable database, for one — without standing up PostgreSQL.
type Server struct {
	cfg          config.Config
	log          *slog.Logger
	queries      sqlcgen.Querier
	tokens       *auth.TokenService
	cookies      auth.CookieWriter
	mailer       email.Sender
	buildVersion string
}

// NewServer wires the dependencies a handler set needs.
func NewServer(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, mailer email.Sender, buildVersion string) *Server {
	return NewServerWithQuerier(cfg, log, sqlcgen.New(pool), mailer, buildVersion)
}

// NewServerWithQuerier builds a Server over any Querier. Used by tests.
func NewServerWithQuerier(cfg config.Config, log *slog.Logger, q sqlcgen.Querier, mailer email.Sender, buildVersion string) *Server {
	return &Server{
		cfg:          cfg,
		log:          log,
		queries:      q,
		tokens:       auth.NewTokenService(cfg.JWTSecret),
		cookies:      auth.NewCookieWriter(cfg.IsDevelopment()),
		mailer:       mailer,
		buildVersion: buildVersion,
	}
}

// Tokens exposes the token service so the router can build the authentication
// middleware from the same instance the handlers mint with.
func (s *Server) Tokens() *auth.TokenService { return s.tokens }

// NewRouter builds the gin engine with the middleware chain and mounts the
// generated routes.
func NewRouter(cfg config.Config, log *slog.Logger, srv *Server) *gin.Engine {
	if !cfg.IsDevelopment() {
		gin.SetMode(gin.ReleaseMode)
	}

	// gin.New rather than gin.Default: the default engine installs its own
	// logger and recovery, which would write responses that do not match the
	// documented error envelope.
	r := gin.New()

	// The generated strict handlers receive the *gin.Context as their
	// context.Context. Without this, gin.Context.Value does NOT fall through to
	// the request's context, so anything middleware attaches there — the
	// authenticated principal, for one — is invisible to handlers and every
	// authenticated request looks anonymous.
	r.ContextWithFallback = true
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.Logger(log),
		middleware.CORS(cfg.CORSOrigin),
		// Populates the principal when a valid session cookie is present, and
		// never rejects. Handlers decide what requires a session.
		middleware.Authenticate(srv.Tokens()),
		// Rejects unsafe methods that carry a session cookie without a matching
		// CSRF header. Runs before any handler sees the request.
		middleware.CSRF(),
	)

	// An unknown path and an unknown record should look the same to a client.
	r.NoRoute(func(c *gin.Context) {
		apierror.NotFound(c, "No endpoint at "+c.Request.URL.Path)
	})
	r.NoMethod(func(c *gin.Context) {
		apierror.Write(c, http.StatusMethodNotAllowed, apierror.CodeBadRequest,
			c.Request.Method+" is not allowed on "+c.Request.URL.Path)
	})

	// oapi-codegen's default error handlers answer with {"msg": "..."}, which is
	// not the documented envelope, and the handler-error default puts the raw Go
	// error in the response body. Both are replaced here so that every failure —
	// including one thrown by generated request binding — leaves through the one
	// shape the contract describes.
	handler := gen.NewStrictHandlerWithOptions(srv, nil, gen.StrictGinServerOptions{
		RequestErrorHandlerFunc: func(c *gin.Context, err error) {
			apierror.Write(c, http.StatusBadRequest, apierror.CodeBadRequest, requestErrorMessage(err))
		},
		ResponseErrorHandlerFunc: func(c *gin.Context, err error) {
			log.Error("writing response failed",
				slog.String("path", c.Request.URL.Path),
				slog.String("error", err.Error()))
		},
		HandlerErrorFunc: func(c *gin.Context, err error) {
			// Logged in full, never returned: an internal error can name tables
			// and columns, and a client has no use for it.
			log.Error("handler failed",
				slog.String("path", c.Request.URL.Path),
				slog.String("request_id", c.GetString(apierror.RequestIDKey)),
				slog.String("error", err.Error()))
			apierror.Internal(c)
		},
	})

	// Path and query parameter binding fails through GinServerOptions.ErrorHandler,
	// which is a different hook from the strict handler's RequestErrorHandlerFunc
	// above — that one only covers the request *body*. Both have to be replaced
	// or a malformed path parameter escapes the documented envelope.
	gen.RegisterHandlersWithOptions(r, handler, gen.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, status int) {
			if status == 0 {
				status = http.StatusBadRequest
			}
			apierror.Write(c, status, apierror.CodeBadRequest, parameterErrorMessage(err))
		},
	})
	return r
}

// parameterErrorMessage explains a bad path or query parameter without echoing
// Go type names at the caller.
func parameterErrorMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "uuid") || strings.Contains(msg, "UUID"):
		return "That id is not a valid UUID."
	case strings.Contains(msg, "Invalid format for parameter"):
		return "A parameter in the request has the wrong format."
	default:
		return "The request parameters could not be read. Check them against the API contract."
	}
}

// requestErrorMessage turns a binding failure into something a person can act
// on, without echoing Go type names back at them.
func requestErrorMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "EOF"):
		return "A JSON body is required."
	case strings.Contains(msg, "email"):
		return "That email address is not valid."
	default:
		return "The request body could not be read. Check the field types against the API contract."
	}
}
