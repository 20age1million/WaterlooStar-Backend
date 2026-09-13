// Command api runs the WaterlooStar HTTP service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/20age1million/waterloostar-api/internal/config"
	"github.com/20age1million/waterloostar-api/internal/db"
	"github.com/20age1million/waterloostar-api/internal/httpapi"
)

// buildVersion is stamped at link time:
//
//	go build -ldflags "-X main.buildVersion=$(git rev-parse --short HEAD)" ./cmd/api
var buildVersion string

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		// Configuration errors are multi-line and meant to be read, so they go
		// out plainly rather than as a structured log line.
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	// Interrupt cancels this context, which unwinds the whole startup chain.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           httpapi.NewRouter(cfg, log, httpapi.NewServer(cfg, log, pool, buildVersion)),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("listening",
			slog.String("addr", cfg.Addr()),
			slog.String("environment", string(cfg.Environment)),
			slog.String("cors_origin", cfg.CORSOrigin),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// Drain in-flight requests before closing the pool, so nothing is cut off
	// mid-query.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", slog.String("error", err.Error()))
		return err
	}

	log.Info("stopped")
	return nil
}
