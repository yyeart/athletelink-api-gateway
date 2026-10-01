package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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

	coreURL, err := url.Parse(cfg.CoreURL)
	if err != nil {
		return fmt.Errorf("parse Core URL: %w", err)
	}

	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return fmt.Errorf("parse Auth URL: %w", err)
	}

	gameURL, err := url.Parse(cfg.GameURL)
	if err != nil {
		return fmt.Errorf("parse Game URL: %w", err)
	}

	coreProxy, err := proxy.NewReverseProxy(coreURL, proxy.RewriteCore, proxy.HandleUpstreamError)
	if err != nil {
		return fmt.Errorf("create Core proxy: %w", err)
	}

	authProxy, err := proxy.NewReverseProxy(authURL, proxy.RewriteAuth, proxy.HandleUpstreamError)
	if err != nil {
		return fmt.Errorf("create Auth proxy: %w", err)
	}

	gameProxy, err := proxy.NewReverseProxy(gameURL, proxy.RewriteGame, proxy.HandleUpstreamError)
	if err != nil {
		return fmt.Errorf("create Game proxy: %w", err)
	}

	logger := observability.NewLogger(os.Stdout, cfg.LogLevel)
	redisClient := redis.NewClient(
		&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
		},
	)
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Error("redis_client_close_failed", "error", err)
		}
	}()
	logger.Info("gateway_configured",
		"http_addr", cfg.HTTPAddr,
		"core_origin", coreURL.String(), "auth_origin", authURL.String(),
		"game_origin", gameURL.String(),
		"redis_addr", redisClient.Options().Addr, "redis_db", redisClient.Options().DB,
		"log_level", cfg.LogLevel.String())

	verifier, err := auth.NewVerifier(
		cfg.JWTSecret, auth.NewRedisDenylist(redisClient), time.Now,
	)
	if err != nil {
		return fmt.Errorf("configure access-token verifier: %w", err)
	}

	checkRedis := func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	}
	checkDependency := newReadinessCheck(cfg, checkRedis)

	return runHTTPServer(
		ctx, cfg,
		coreProxy, authProxy, gameProxy,
		verifier, checkDependency,
		func() string {
			return uuid.New().String()
		}, logger)
}

func runHTTPServer(
	ctx context.Context,
	cfg config.Config,
	coreProxy http.Handler,
	authProxy http.Handler,
	gameProxy http.Handler,
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
		AuthHandler:     authProxy,
		GameHandler:     gameProxy,
		CoreTimeout:     cfg.CoreTimeout,
		AuthTimeout:     cfg.AuthTimeout,
		GameTimeout:     cfg.GameTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		Verifier:        verifier,
		NewRequestID:    newRequestID,
		CORSOrigins:     cfg.CORSOrigins,
		Logger:          logger,
	})

	return server.New(cfg, handler, readiness.Set, logger).Run(ctx)
}
