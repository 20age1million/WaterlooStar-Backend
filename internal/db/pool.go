// Package db owns the PostgreSQL connection pool and the sqlc-generated queries
// that run against it.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool limits. Modest on purpose: the service is expected to run as a small
// number of replicas against a single Postgres, and an oversized pool starves
// the database rather than helping.
const (
	maxConns        = 25
	minConns        = 2
	maxConnLifetime = 2 * time.Hour
	maxConnIdleTime = 15 * time.Minute
	connectTimeout  = 10 * time.Second
)

// Open parses the DSN, opens a pool, and verifies the database is reachable
// before returning. Failing here rather than on the first request means a
// misconfigured deployment never reports itself as started.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PG_DSN: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unreachable — is `docker compose up -d` running? %w", err)
	}

	return pool, nil
}
