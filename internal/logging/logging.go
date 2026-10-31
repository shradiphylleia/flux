// Package logging builds the structured JSON logger shared by all binaries.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(level, service string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	return slog.New(h).With("service", service)
}
