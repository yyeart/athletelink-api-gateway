package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/auth"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/grpcapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/httpapi"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/observability"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/server"
	"google.golang.org/grpc"
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

	if cfg.CoreURL == "" {
		return errors.New("GATEWAY_CORE_URL is required for gRPC")
	}

	coreURL, err := url.Parse(cfg.CoreURL)
	if err != nil {
		return fmt.Errorf("parse Core URL: %w", err)
	}

	core, err := coreclient.New(coreURL, &http.Client{
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create Core client: %w", err)
	}

	redisClient := redis.NewClient(
		&redis.Options{
			Addr: cfg.RedisAddr,
		},
	)
	defer func() {
		_ = redisClient.Close()
	}()

	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := redisClient.Ping(pingCtx).Result(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("redis client ping deadline exceeded: %w", err)
		}

		return fmt.Errorf("redis client ping error: %w", err)
	}

	verifier, err := auth.NewVerifier(
		cfg.JWTSecret, auth.NewRedisDenylist(redisClient), time.Now,
	)
	if err != nil {
		return fmt.Errorf("configure access-token verifier: %w", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	logger := observability.NewLogger(os.Stdout, cfg.LogLevel)
	checkRedis := func(ctx context.Context) error {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return redisClient.Ping(pingCtx).Err()
	}
	return runServers(ctx, cfg, core, verifier, checkRedis, func() string {
		return uuid.New().String()
	}, logger)
}

func runServers(
	ctx context.Context,
	cfg config.Config,
	core grpcapi.Core,
	verifier grpcapi.AccessVerifier,
	checkDependency func(context.Context) error,
	newRequestID func() string,
	logger *slog.Logger,
) error {
	readiness := &httpapi.Readiness{}
	var readinessMu sync.Mutex
	httpReady, grpcReady := false, false
	setReady := func(httpListener, ready bool) {
		readinessMu.Lock()
		defer readinessMu.Unlock()
		if httpListener {
			httpReady = ready
		} else {
			grpcReady = ready
		}
		readiness.Set(httpReady && grpcReady)
	}
	setHTTPReady := func(ready bool) { setReady(true, ready) }
	setGRPCReady := func(ready bool) { setReady(false, ready) }
	httpServer := server.New(
		cfg, httpapi.NewHandler(readiness, checkDependency),
		setHTTPReady, logger,
	)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcapi.AuthInterceptor(verifier, newRequestID)),
		grpcapi.UnknownServiceHandler(newRequestID),
	)
	gatewayv1.RegisterCoreServiceServer(grpcServer, grpcapi.New(core))

	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC on %s: %w", cfg.GRPCAddr, err)
	}
	defer func() {
		_ = listener.Close() //nolint:errcheck
	}()
	setGRPCReady(true)
	defer setGRPCReady(false)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 2)

	go func() {
		results <- result{"HTTP", httpServer.Run(runCtx)}
	}()
	go func() {
		err := grpcServer.Serve(listener)
		setGRPCReady(false)
		results <- result{"gRPC", err}
	}()

	var first *result
	select {
	case r := <-results:
		first = &r

	case <-ctx.Done():
	}
	cancel()
	setGRPCReady(false)

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	timer := time.NewTimer(cfg.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-stopped:
	case <-timer.C:
		grpcServer.Stop()
		<-stopped
	}

	completed := 0
	var runErr error
	check := func(r result) {
		if r.err != nil &&
			!(r.name == "gRPC" && errors.Is(r.err, grpc.ErrServerStopped)) {
			runErr = errors.Join(runErr, fmt.Errorf("%s server: %w", r.name, r.err))
		}
	}
	if first != nil {
		check(*first)
		completed++
	}
	for completed < 2 {
		check(<-results)
		completed++
	}
	return runErr
}
