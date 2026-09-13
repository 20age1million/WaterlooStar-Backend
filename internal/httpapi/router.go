// Package httpapi builds the gin router and implements the handlers behind the
// interface generated from api/openapi.yaml.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/20age1million/waterloostar-api/internal/apierror"
	"github.com/20age1million/waterloostar-api/internal/config"
	"github.com/20age1million/waterloostar-api/internal/db/sqlcgen"
	"github.com/20age1million/waterloostar-api/internal/httpapi/gen"
	"github.com/20age1million/waterloostar-api/internal/middleware"
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
	buildVersion string
}

// NewServer wires the dependencies a handler set needs.
func NewServer(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, buildVersion string) *Server {
	return NewServerWithQuerier(cfg, log, sqlcgen.New(pool), buildVersion)
}

// NewServerWithQuerier builds a Server over any Querier. Used by tests.
func NewServerWithQuerier(cfg config.Config, log *slog.Logger, q sqlcgen.Querier, buildVersion string) *Server {
	return &Server{
		cfg:          cfg,
		log:          log,
		queries:      q,
		buildVersion: buildVersion,
	}
}

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
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.Logger(log),
		middleware.CORS(cfg.CORSOrigin),
	)

	// An unknown path and an unknown record should look the same to a client.
	r.NoRoute(func(c *gin.Context) {
		apierror.NotFound(c, "No endpoint at "+c.Request.URL.Path)
	})
	r.NoMethod(func(c *gin.Context) {
		apierror.Write(c, http.StatusMethodNotAllowed, apierror.CodeBadRequest,
			c.Request.Method+" is not allowed on "+c.Request.URL.Path)
	})

	gen.RegisterHandlers(r, gen.NewStrictHandler(srv, nil))
	return r
}
