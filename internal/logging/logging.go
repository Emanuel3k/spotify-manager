// Package logging builds the application's single *slog.Logger. Every
// layer (services and adapters) receives this logger through its
// constructor so login attempts, HTTP calls to Spotify and their outcomes
// can all be traced from one place instead of being scattered as ad-hoc
// fmt.Println calls.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a text logger writing to stderr (stdout is reserved for the
// CLI's actual output/results). Level is read from LOG_LEVEL
// (debug|info|warn|error), defaulting to info.
func New() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
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
