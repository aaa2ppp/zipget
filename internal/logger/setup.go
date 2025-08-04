package logger

import (
	"log/slog"
	"os"

	"2025-07-30/internal/config"
)

func SetupDefault(cfg config.Logger) {
	if cfg.Plaintext {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Level})))
	} else {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Level})))
	}
}
