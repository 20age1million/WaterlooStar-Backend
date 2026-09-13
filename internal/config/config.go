// Package config binds and validates the service's environment configuration.
//
// Everything is read once at startup and validated together, so a misconfigured
// service fails immediately with a readable message instead of producing a nil
// pool or an empty URL somewhere deep in a request.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Environment distinguishes local development from a deployed service. It gates
// behaviour that must not be relaxed in production, such as the Secure flag on
// the session cookie introduced in Phase 1.
type Environment string

const (
	Development Environment = "development"
	Production  Environment = "production"
)

// minJWTSecretBytes is the shortest secret accepted. HS256 keys shorter than the
// hash output add nothing, and a short one here is usually a placeholder that
// reached production by accident.
const minJWTSecretBytes = 32

// Config is the validated configuration for one process.
type Config struct {
	DatabaseURL string
	Port        int
	CORSOrigin  string
	LogLevel    slog.Level
	Environment Environment
	// JWTSecret signs access tokens. Rotating it invalidates every live session.
	JWTSecret string
	// AppURL is the frontend's public origin, used to build the links that go
	// into verification and password-reset emails.
	AppURL string
}

// IsDevelopment reports whether the service is running locally.
func (c Config) IsDevelopment() bool { return c.Environment == Development }

// Addr is the listen address for the HTTP server.
func (c Config) Addr() string { return fmt.Sprintf(":%d", c.Port) }

// Load reads configuration from the environment, falling back to a .env file
// when one is present. It returns an error naming every problem it found rather
// than only the first, so a fresh checkout can be fixed in one pass.
func Load() (Config, error) {
	// A missing .env is normal in a deployed environment.
	_ = godotenv.Load()

	var cfg Config
	var problems []string

	cfg.DatabaseURL = strings.TrimSpace(os.Getenv("PG_DSN"))
	if cfg.DatabaseURL == "" {
		problems = append(problems, `PG_DSN is not set — copy .env.example to .env, or export it:
    PG_DSN=postgres://waterloostar:waterloostar@localhost:5433/waterloostar?sslmode=disable`)
	}

	cfg.Port = 8080
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			problems = append(problems, fmt.Sprintf("PORT %q is not a valid port number", raw))
		} else {
			cfg.Port = port
		}
	}

	cfg.CORSOrigin = strings.TrimSpace(os.Getenv("CORS_ORIGIN"))
	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "http://localhost:3000"
	}

	cfg.LogLevel = slog.LevelInfo
	if raw := strings.TrimSpace(os.Getenv("LOG_LEVEL")); raw != "" {
		level, err := parseLevel(raw)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			cfg.LogLevel = level
		}
	}

	cfg.Environment = Development
	if raw := strings.TrimSpace(os.Getenv("ENVIRONMENT")); raw != "" {
		switch Environment(strings.ToLower(raw)) {
		case Development:
			cfg.Environment = Development
		case Production:
			cfg.Environment = Production
		default:
			problems = append(problems, fmt.Sprintf("ENVIRONMENT %q must be development or production", raw))
		}
	}

	cfg.JWTSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	switch {
	case cfg.JWTSecret == "":
		problems = append(problems, `JWT_SECRET is not set — generate one:
    openssl rand -base64 48`)
	case len(cfg.JWTSecret) < minJWTSecretBytes:
		problems = append(problems, fmt.Sprintf(
			"JWT_SECRET is %d bytes; at least %d are required", len(cfg.JWTSecret), minJWTSecretBytes))
	}

	cfg.AppURL = strings.TrimSpace(os.Getenv("APP_URL"))
	if cfg.AppURL == "" {
		cfg.AppURL = cfg.CORSOrigin
	}
	cfg.AppURL = strings.TrimRight(cfg.AppURL, "/")

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL %q must be one of debug, info, warn, error", raw)
	}
}
