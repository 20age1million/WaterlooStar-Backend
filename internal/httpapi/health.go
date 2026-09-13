package httpapi

import (
	"context"
	"log/slog"
	"time"

	"github.com/20age1million/waterloostar-api/internal/httpapi/gen"
)

// healthTimeout bounds the database check so an unresponsive database produces a
// prompt 503 rather than holding the connection open until the client gives up.
const healthTimeout = 2 * time.Second

// GetHealth reports service and database state.
//
// The database check is a real round-trip, not a cached flag: the endpoint exists
// so a load balancer can stop sending traffic to an instance that cannot serve it,
// and a cached answer would defeat that.
func (s *Server) GetHealth(ctx context.Context, _ gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	checkCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	if _, err := s.queries.Ping(checkCtx); err != nil {
		s.log.Error("health check failed", slog.String("error", err.Error()))
		return gen.GetHealth503JSONResponse{
			Status:   gen.HealthStatusDegraded,
			Database: gen.HealthDatabaseUnreachable,
			Version:  s.version(),
		}, nil
	}

	return gen.GetHealth200JSONResponse{
		Status:   gen.HealthStatusOk,
		Database: gen.HealthDatabaseOk,
		Version:  s.version(),
	}, nil
}

func (s *Server) version() *string {
	if s.buildVersion == "" {
		return nil
	}
	return &s.buildVersion
}

// compile-time proof that Server satisfies the generated contract.
var _ gen.StrictServerInterface = (*Server)(nil)
