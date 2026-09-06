package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
)

type Server struct {
	http            *http.Server
	shutdownTimeout time.Duration
	setReady        func(bool)
	logger          *slog.Logger
}

func New(
	cfg config.Config,
	handler http.Handler,
	setReady func(bool),
	logger *slog.Logger,
) *Server {
	if handler == nil {
		panic("server.New: nil HTTP handler")
	}

	if setReady == nil {
		panic("server.New: nil readiness callback")
	}

	if logger == nil {
		panic("server.New: nil logger")
	}

	serverLogger := logger.With("component", "http_server")

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog: slog.NewLogLogger(
			serverLogger.Handler(),
			slog.LevelError,
		),
	}

	return &Server{
		http:            httpServer,
		shutdownTimeout: cfg.ShutdownTimeout,
		setReady:        setReady,
		logger:          serverLogger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.http.Addr, err)
	}

	s.setReady(true)
	defer s.setReady(false)

	serveErrCh := make(chan error, 1)

	go func() {
		defer close(serveErrCh)

		s.logger.Warn("serve HTTP server", "addr", s.http.Addr)

		err := s.http.Serve(listener)

		if !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	select {
	case err := <-serveErrCh:
		if err != nil {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		s.logger.Warn("shutdown HTTP server")

		s.setReady(false)

		shutdownCtx := context.WithoutCancel(ctx)
		shutdownCtx, cancel := context.WithTimeout(
			shutdownCtx, s.shutdownTimeout,
		)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			_ = s.http.Close() //nolint:errcheck // rollback is best effort after the operation result is known

			return fmt.Errorf("shutdown HTTP server: %w", err)
		}

		s.logger.Warn("HTTP server stopped")
	}

	return nil
}
