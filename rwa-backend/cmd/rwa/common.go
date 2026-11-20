package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
)

// newLogger builds the process logger (LOG_LEVEL, LOG_FORMAT) writing to w: JSON unless LOG_FORMAT=text.
func newLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	return slog.New(newBaseHandler(cfg, w))
}

// newJournalLogger is newLogger whose records that carry a run id are also stored in run_events (RUN_EVENTS_LEVEL). The
// events are written once the caller sets the sink (h.SetSink(store.New(pool))) and the pool exists; defer h.Close()
// after the pool's deferred close, so the last events are flushed while the pool is still open.
func newJournalLogger(cfg *config.Config, w io.Writer) (*slog.Logger, *runlog.Handler) {
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.RunEventsLevel)) // validated by Config.Validate*
	h := runlog.New(newBaseHandler(cfg, w), runlog.Options{Level: level})
	return slog.New(h), h
}

func newBaseHandler(cfg *config.Config, w io.Writer) slog.Handler {
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel)) // validated by Config.Validate*
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
			a.Value = slog.TimeValue(a.Value.Time().UTC())
		}
		return a
	}}
	if cfg.LogFormat == "text" {
		return slog.NewTextHandler(w, opts)
	}
	return slog.NewJSONHandler(w, opts)
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
