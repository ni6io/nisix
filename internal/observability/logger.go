package observability

import (
	"log/slog"
	"os"
)

func New(service string) *slog.Logger {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h).With("service", service)
}
