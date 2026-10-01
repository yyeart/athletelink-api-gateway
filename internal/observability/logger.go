package observability

import (
	"io"
	"log/slog"
)

func NewLogger(out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: level,
	})).With("service", "api-gateway")
}
