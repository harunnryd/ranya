package logging

import (
	"log/slog"
	"os"
)

// InitLogger initializes a global logger with the specified level.
// It configures a JSON handler with source location information.
func InitLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	return slog.New(handler)
}

// NewComponentLogger creates a component-specific logger with context.
// It adds the component name to all log messages for better traceability.
func NewComponentLogger(base *slog.Logger, component string) *slog.Logger {
	return base.With(
		slog.String("component", component),
	)
}
