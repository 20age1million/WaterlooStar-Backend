package httpapi

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/config"
)

// The allowlist's failure mode is silence.
//
// A path that does not match a registered route limits nothing, and nothing
// fails: the endpoint simply stays unlimited, and the only way to find out is an
// attack. These tests are in-package because the route table is, deliberately —
// it is not something another package should be able to edit.

func routerPaths(t *testing.T) map[string]map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := config.Config{
		Port:        8080,
		Environment: config.Development,
		JWTSecret:   "a-test-secret-that-is-long-enough-to-pass",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewRouter(cfg, log, NewServerWithQuerier(cfg, log, nil, nil, ""))

	paths := map[string]map[string]bool{}
	for _, route := range r.Routes() {
		if paths[route.Method] == nil {
			paths[route.Method] = map[string]bool{}
		}
		paths[route.Method][route.Path] = true
	}
	return paths
}

func TestEveryLimitedRouteExists(t *testing.T) {
	paths := routerPaths(t)

	for _, route := range limitedRoutes {
		if !paths[route.method][route.path] {
			t.Errorf("%s %s is rate limited but is not a route — the limit does nothing",
				route.method, route.path)
		}
	}
}

func TestEveryLimitedRouteHasARuleAndAMessage(t *testing.T) {
	for _, route := range limitedRoutes {
		if route.perKey.Burst <= 0 || route.perKey.Window <= 0 {
			t.Errorf("%s %s has no usable per-identity rule, so it permits everything",
				route.method, route.path)
		}
		if route.message == "" {
			t.Errorf("%s %s has no refusal message", route.method, route.path)
		}
		// Every message carries the wait, because "try again later" without a
		// number is the thing users complain about.
		if !containsVerb(route.message) {
			t.Errorf("%s %s message does not name the wait: %q",
				route.method, route.path, route.message)
		}
	}
}

func containsVerb(message string) bool {
	for i := range len(message) - 1 {
		if message[i] == '%' && message[i+1] == 's' {
			return true
		}
	}
	return false
}

func TestNoRouteIsListedTwice(t *testing.T) {
	// Two entries for one route would mean the second is dead, and whichever
	// rule someone later edited might be the one that never runs.
	seen := map[string]bool{}
	for _, route := range limitedRoutes {
		key := route.method + " " + route.path
		if seen[key] {
			t.Errorf("%s appears twice in the allowlist", key)
		}
		seen[key] = true
	}
}

func TestRefreshIsNotInTheAllowlist(t *testing.T) {
	// Recorded as a decision, not an omission: the frontend refreshes on
	// navigation, so limiting this signs people out for browsing.
	for _, route := range limitedRoutes {
		if route.path == "/auth/refresh" {
			t.Error("/auth/refresh is rate limited — session refresh will sign users out")
		}
	}
}
