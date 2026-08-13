// Package logging builds the process-wide slog.Logger.
//
// Prod uses JSON so a log pipeline (Loki, ELK, Datadog) can index fields.
// Dev uses text with source locations so a human can read it in a terminal.
package logging

import (
	"log/slog"
	"os"
)

// New returns a logger configured for the given environment.
// env == "production" -> JSON handler at Info level.
// anything else       -> Text handler at Debug level with source line.
func New(env string) *slog.Logger {
	if env == "production" {
		h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		return slog.New(h)
	}

	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})
	return slog.New(h)
}
