package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
)

// newLogger builds the process logger (LOG_LEVEL, LOG_FORMAT) writing to w: JSON unless LOG_FORMAT=text.
func newLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel)) // validated by Config.Validate*
	opts := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// ensureMigrated fails when the database has pending migrations.
func ensureMigrated(ctx context.Context, dbURL string) error {
	st, err := db.Status(ctx, dbURL)
	if err != nil {
		return err
	}
	for _, m := range st {
		if !m.Applied {
			return fmt.Errorf("pending migration %04d %s: run `rwa migrate up` first", m.Version, m.Source)
		}
	}
	return nil
}
