package config_test

import (
	"log/slog"
	"reflect"
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
		CoreTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		LogLevel:          slog.LevelInfo,
		RedisAddr:         "localhost:6379",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Parallel()

	env := environment{
		"GATEWAY_HTTP_ADDR":           "127.0.0.1:9090",
		"GATEWAY_CORE_URL":            "https://core.example/",
		"GATEWAY_CORS_ORIGINS":        "https://app.example, http://localhost:3000",
		"GATEWAY_CORE_TIMEOUT":        "2s",
		"GATEWAY_READ_HEADER_TIMEOUT": "1s",
		"GATEWAY_READ_TIMEOUT":        "2s",
		"GATEWAY_WRITE_TIMEOUT":       "3s",
		"GATEWAY_IDLE_TIMEOUT":        "4s",
		"GATEWAY_SHUTDOWN_TIMEOUT":    "5s",
		"GATEWAY_LOG_LEVEL":           "debug",
		"REDIS_ADDR":                  "127.0.0.1:6380",
		"REDIS_PASSWORD":              "test-only-redis-password",
		"JWT_SECRET":                  "test-only-secret-not-for-production",
	}

	got, err := config.Load(env.lookup)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		HTTPAddr:          "127.0.0.1:9090",
		CoreURL:           "https://core.example/",
		CORSOrigins:       []string{"https://app.example", "http://localhost:3000"},
		CoreTimeout:       2 * time.Second,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		LogLevel:          slog.LevelDebug,
		RedisAddr:         "127.0.0.1:6380",
		RedisPassword:     "test-only-redis-password",
		JWTSecret:         "test-only-secret-not-for-production",
	}

	if !reflect.DeepEqual(got, want) {
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

	for _, key := range []string{"GATEWAY_HTTP_ADDR"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			assertLoadError(t, environment{key: " \t"}, key)
		})
	}
}

func TestLoadRejectsInvalidCORSOrigins(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"", " ", "*", "app.example", "ftp://app.example",
		"https://app.example/", "https://app.example/path",
		"https://app.example?key=value", "https://app.example#fragment",
		"https://user:pass@app.example", "https://app.example,",
		"https://app.example, https://app.example",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			assertLoadError(t, environment{"GATEWAY_CORS_ORIGINS": value}, "GATEWAY_CORS_ORIGINS")
		})
	}
}

func TestLoadRejectsInvalidCoreTimeout(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"later", "0s", "-1s", "15s", "16s"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			assertLoadError(t, environment{"GATEWAY_CORE_TIMEOUT": value}, "GATEWAY_CORE_TIMEOUT")
		})
	}
}

func TestLoadComparesCoreTimeoutWithOverriddenWriteTimeout(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"3s", "10s"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			assertLoadError(t, environment{"GATEWAY_WRITE_TIMEOUT": value}, "GATEWAY_CORE_TIMEOUT")
		})
	}

	cfg, err := config.Load(environment{
		"GATEWAY_CORE_TIMEOUT":  "2s",
		"GATEWAY_WRITE_TIMEOUT": "3s",
	}.lookup)
	if err != nil {
		t.Fatalf("Load() with Core timeout below write timeout: %v", err)
	}
	if cfg.CoreTimeout != 2*time.Second || cfg.WriteTimeout != 3*time.Second {
		t.Fatalf("Load() timeouts = (%s, %s), want (2s, 3s)", cfg.CoreTimeout, cfg.WriteTimeout)
	}
}

func TestLoadRejectsEmptyRedisPassword(t *testing.T) {
	t.Parallel()
	assertLoadError(t, environment{"REDIS_PASSWORD": " \t"}, "REDIS_PASSWORD")
}

func TestLoadRejectsInvalidCoreURL(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"", "core:8081", "ftp://core.example", "http://",
		"http://user:pass@core.example", "http://core.example/path",
		"http://core.example?key=value", "http://core.example#fragment",
		"http://core.example?", "http://core.example#",
		" http://core.example ",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			assertLoadError(t, environment{"GATEWAY_CORE_URL": value}, "GATEWAY_CORE_URL")
		})
	}
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
