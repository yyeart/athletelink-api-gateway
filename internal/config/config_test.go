package config_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
)

type environment map[string]string

func (env environment) lookup(key string) (string, bool) {
	value, ok := env[key]

	return value, ok
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(environment{}.lookup)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		HTTPAddr:          ":8080",
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		LogLevel:          slog.LevelInfo,
	}

	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Parallel()

	env := environment{
		"GATEWAY_HTTP_ADDR":           "127.0.0.1:9090",
		"GATEWAY_READ_HEADER_TIMEOUT": "1s",
		"GATEWAY_READ_TIMEOUT":        "2s",
		"GATEWAY_WRITE_TIMEOUT":       "3s",
		"GATEWAY_IDLE_TIMEOUT":        "4s",
		"GATEWAY_SHUTDOWN_TIMEOUT":    "5s",
		"GATEWAY_LOG_LEVEL":           "debug",
	}

	got, err := config.Load(env.lookup)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		HTTPAddr:          "127.0.0.1:9090",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		LogLevel:          slog.LevelDebug,
	}

	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsUnknownLogLevel(t *testing.T) {
	t.Parallel()

	assertLoadError(
		t,
		environment{"GATEWAY_LOG_LEVEL": "verbose"},
		"GATEWAY_LOG_LEVEL",
	)
}

func TestLoadRejectsMalformedDuration(t *testing.T) {
	t.Parallel()

	assertLoadError(
		t,
		environment{"GATEWAY_READ_TIMEOUT": "later"},
		"GATEWAY_READ_TIMEOUT",
	)
}

func TestLoadRejectsZeroDuration(t *testing.T) {
	t.Parallel()

	assertLoadError(
		t,
		environment{"GATEWAY_WRITE_TIMEOUT": "0s"},
		"GATEWAY_WRITE_TIMEOUT",
	)
}

func TestLoadRejectsNegativeDuration(t *testing.T) {
	t.Parallel()

	assertLoadError(
		t,
		environment{"GATEWAY_IDLE_TIMEOUT": "-1s"},
		"GATEWAY_IDLE_TIMEOUT",
	)
}

func TestLoadRejectsEmptyAddress(t *testing.T) {
	t.Parallel()

	assertLoadError(
		t,
		environment{"GATEWAY_HTTP_ADDR": " \t"},
		"GATEWAY_HTTP_ADDR",
	)
}

func assertLoadError(t *testing.T, env environment, wantSubstring string) {
	t.Helper()

	_, err := config.Load(env.lookup)
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil error")
	}

	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("Load() error = %q, want substring %q", err, wantSubstring)
	}
}
