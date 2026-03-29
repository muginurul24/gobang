package observability

import (
	"log/slog"
	"os"
	"strings"

	"github.com/mugiew/onixggr/internal/platform/config"
)

func NewLogger(cfg config.AppConfig) *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(parseLevel(cfg.LogLevel))

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
