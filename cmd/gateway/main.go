package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/auth"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/observability"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/proxy"
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
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	return runWithContext(ctx)
}

func runWithContext(ctx context.Context) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	if cfg.CoreURL == "" {
		return errors.New("GATEWAY_CORE_URL is required")
	}

	coreURL, err := url.Parse(cfg.CoreURL)
	if err != nil {
		return fmt.Errorf("parse Core URL: %w", err)
	}

	coreProxy, err := proxy.NewReverseProxy(coreURL, proxy.RewriteCore, proxy.HandleCoreError)
	if err != nil {
		return fmt.Errorf("create core proxy: %w", err)
	}

	coreProxy.ModifyResponse = func(resp *http.Response) error {
		for name := range resp.Header {
			if strings.EqualFold(name, "X-Request-Id") ||
				strings.HasPrefix(strings.ToLower(name), "access-control-") {
				delete(resp.Header, name)
			}
		}

		return nil
	}

	logger := observability.NewLogger(os.Stdout, cfg.LogLevel)
	redisClient := redis.NewClient(
		&redis.Options{
			Addr: cfg.RedisAddr,
		},
	)
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Error("redis_client_close_failed", "error", err)
		}
	}()

	verifier, err := auth.NewVerifier(
		cfg.JWTSecret, auth.NewRedisDenylist(redisClient), time.Now,
	)
	if err != nil {
		return fmt.Errorf("configure access-token verifier: %w", err)
	}

	checkRedis := func(ctx context.Context) error {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return redisClient.Ping(pingCtx).Err()
	}

	return runHTTPServer(ctx, cfg, coreProxy, verifier, checkRedis, func() string {
		return uuid.New().String()
	}, logger)
}

func runHTTPServer(
	ctx context.Context,
	cfg config.Config,
	coreProxy http.Handler,
	verifier httpapi.AccessVerifier,
	checkDependency func(context.Context) error,
	newRequestID func() string,
	logger *slog.Logger,
) error {
	readiness := &httpapi.Readiness{}
	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Readiness:       readiness,
		CheckDependency: checkDependency,
		CoreProxy:       coreProxy,
		CoreTimeout:     cfg.CoreTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		Verifier:        verifier,
		NewRequestID:    newRequestID,
		CORSOrigins:     cfg.CORSOrigins,
	})

	return server.New(cfg, handler, readiness.Set, logger).Run(ctx)
}
