package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

type LookupFunc func(string) (string, bool)

type Config struct {
	HTTPAddr          string
	GRPCAddr          string
	CoreURL           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	LogLevel          slog.Level
	RedisAddr         string
	JWTSecret         string
}

var defaultConfig = Config{
	HTTPAddr:          ":8080",
	GRPCAddr:          ":9090",
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       15 * time.Second,
	WriteTimeout:      15 * time.Second,
	IdleTimeout:       60 * time.Second,
	ShutdownTimeout:   10 * time.Second,
	LogLevel:          slog.LevelInfo,
	RedisAddr:         "localhost:6379",
}

func Load(lookup LookupFunc) (Config, error) {
	cfg := defaultConfig

	for _, address := range []struct {
		key    string
		target *string
	}{
		{"GATEWAY_HTTP_ADDR", &cfg.HTTPAddr},
		{"GATEWAY_GRPC_ADDR", &cfg.GRPCAddr},
	} {
		if err := overrideNonEmpty(lookup, address.key, address.target); err != nil {
			return Config{}, err
		}
	}
	if value, ok := lookup("GATEWAY_CORE_URL"); ok {
		if err := validateCoreURL(value); err != nil {
			return Config{}, fmt.Errorf("GATEWAY_CORE_URL: %w", err)
		}

		cfg.CoreURL = value
	}

	var err error

	cfg.ReadHeaderTimeout, err = positiveDuration(
		lookup, "GATEWAY_READ_HEADER_TIMEOUT", cfg.ReadHeaderTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.ReadTimeout, err = positiveDuration(
		lookup, "GATEWAY_READ_TIMEOUT", cfg.ReadTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.WriteTimeout, err = positiveDuration(
		lookup, "GATEWAY_WRITE_TIMEOUT", cfg.WriteTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.IdleTimeout, err = positiveDuration(
		lookup, "GATEWAY_IDLE_TIMEOUT", cfg.IdleTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.ShutdownTimeout, err = positiveDuration(
		lookup, "GATEWAY_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.LogLevel, err = logLevel(
		lookup, "GATEWAY_LOG_LEVEL", cfg.LogLevel,
	)
	if err != nil {
		return Config{}, err
	}

	if err := overrideNonEmpty(lookup, "REDIS_ADDR", &cfg.RedisAddr); err != nil {
		return Config{}, err
	}

	if err := overrideNonEmpty(lookup, "JWT_SECRET", &cfg.JWTSecret); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func overrideNonEmpty(lookup LookupFunc, key string, target *string) error {
	value, ok := lookup(key)
	if !ok {
		return nil
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: must not be empty", key)
	}
	*target = value
	return nil
}

func validateCoreURL(value string) error {
	if value == "" || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "?#") {
		return errors.New("must be an HTTP(S) URL with a host")
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || !validCoreURL(parsed) {
		return errors.New("must be an HTTP(S) URL with a host and no credentials, path, query, or fragment")
	}

	return nil
}

func validCoreURL(parsed *url.URL) bool {
	return validCoreURLAuthority(parsed) && validCoreURLPath(parsed)
}

func validCoreURLAuthority(parsed *url.URL) bool {
	return (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Hostname() != "" && parsed.User == nil && parsed.Opaque == ""
}

func validCoreURLPath(parsed *url.URL) bool {
	return (parsed.Path == "" || parsed.Path == "/") &&
		parsed.RawPath == "" && parsed.RawQuery == "" && !parsed.ForceQuery &&
		parsed.Fragment == "" && parsed.RawFragment == ""
}

func positiveDuration(
	lookup LookupFunc,
	key string,
	fallback time.Duration,
) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(
			"%s: invalid duration: %q: %w",
			key, value, err,
		)
	}

	if duration <= 0 {
		return 0, fmt.Errorf(
			"%s: must be positive, got %q",
			key, value,
		)
	}

	return duration, nil

}

func logLevel(
	lookup LookupFunc,
	key string,
	fallback slog.Level,
) (slog.Level, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	switch strings.ToLower(val) {
	case "debug":
		return slog.LevelDebug, nil

	case "info":
		return slog.LevelInfo, nil

	case "warn":
		return slog.LevelWarn, nil

	case "error":
		return slog.LevelError, nil

	default:
		return 0, fmt.Errorf(
			"%s: allowed values: debug, info, warn, error, got %q",
			key, val,
		)
	}
}
