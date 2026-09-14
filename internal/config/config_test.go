package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/config"
)

const validSecret = "a-test-secret-that-is-long-enough-to-pass"

const validDSN = "postgres://waterloostar:waterloostar@localhost:5433/waterloostar?sslmode=disable"

// isolate clears every variable Load reads, so one test's environment cannot
// leak into another's. t.Setenv restores the previous value when the test ends.
func isolate(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PG_DSN", "PORT", "CORS_ORIGIN", "LOG_LEVEL", "ENVIRONMENT", "JWT_SECRET", "APP_URL"} {
		t.Setenv(key, "")
	}
}

func TestLoadFailsWithoutDSN(t *testing.T) {
	isolate(t)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error when PG_DSN is unset, got nil")
	}
	if !strings.Contains(err.Error(), "PG_DSN") {
		t.Errorf("error should name the missing variable, got: %v", err)
	}
	// The message is the first thing a developer sees on a fresh checkout, so it
	// must point at the fix rather than only stating the problem.
	if !strings.Contains(err.Error(), ".env.example") {
		t.Errorf("error should point at .env.example, got: %v", err)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	isolate(t)
	t.Setenv("PG_DSN", validDSN)
	t.Setenv("JWT_SECRET", validSecret)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.CORSOrigin != "http://localhost:3000" {
		t.Errorf("CORSOrigin = %q, want the Next.js dev server", cfg.CORSOrigin)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if !cfg.IsDevelopment() {
		t.Error("Environment should default to development")
	}
	if cfg.Addr() != ":8080" {
		t.Errorf("Addr() = %q, want \":8080\"", cfg.Addr())
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   string
		wantSub string
	}{
		{"port not a number", "PORT", "eighty", "PORT"},
		{"port out of range", "PORT", "70000", "PORT"},
		{"unknown log level", "LOG_LEVEL", "chatty", "LOG_LEVEL"},
		{"unknown environment", "ENVIRONMENT", "staging", "ENVIRONMENT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			t.Setenv("PG_DSN", validDSN)
			t.Setenv("JWT_SECRET", validSecret)
			t.Setenv(tc.key, tc.value)

			if _, err := config.Load(); err == nil {
				t.Fatalf("expected an error for %s=%q", tc.key, tc.value)
			} else if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error should name %s, got: %v", tc.wantSub, err)
			}
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	isolate(t)
	t.Setenv("PORT", "nope")
	t.Setenv("LOG_LEVEL", "loud")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	// A fresh checkout with several problems should be fixable in one pass.
	for _, want := range []string{"PG_DSN", "PORT", "LOG_LEVEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s, got: %v", want, err)
		}
	}
}

func TestProductionIsNotDevelopment(t *testing.T) {
	isolate(t)
	t.Setenv("PG_DSN", validDSN)
	t.Setenv("JWT_SECRET", validSecret)
	t.Setenv("ENVIRONMENT", "production")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IsDevelopment() {
		t.Error("IsDevelopment() must be false in production — it gates cookie Secure in Phase 1")
	}
}
