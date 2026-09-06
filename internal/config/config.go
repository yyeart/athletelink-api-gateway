package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type LookupFunc func(string) (string, bool)

type Config struct {
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	LogLevel          slog.Level
}

var defaultConfig = Config{
	HTTPAddr:          ":8080",
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       15 * time.Second,
	WriteTimeout:      15 * time.Second,
	IdleTimeout:       60 * time.Second,
	ShutdownTimeout:   10 * time.Second,
	LogLevel:          slog.LevelInfo,
}

func Load(lookup LookupFunc) (Config, error) {
	cfg := defaultConfig

	if value, ok := lookup("GATEWAY_HTTP_ADDR"); ok {
		if strings.TrimSpace(value) == "" {
			return Config{}, errors.New(
				"GATEWAY_HTTP_ADDR: must not be empty",
			)
		}

		cfg.HTTPAddr = value
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

	return cfg, nil
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
