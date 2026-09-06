package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/observability"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/server"
)

func main() {
	bootstrapLogger := observability.NewLogger(os.Stderr, slog.LevelInfo)

	if err := run(); err != nil {
		bootstrapLogger.Error("gateway_failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := observability.NewLogger(os.Stdout, cfg.LogLevel) // TODO: Add *.log files
	readiness := &httpapi.Readiness{}
	handler := httpapi.NewHandler(readiness)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	srv := server.New(
		cfg, handler, readiness.Set, logger,
	)

	return srv.Run(ctx)
}
