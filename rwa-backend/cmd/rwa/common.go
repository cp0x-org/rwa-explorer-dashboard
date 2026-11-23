package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
)

// loadCurated reads the curated configs (links.json, rights_profiles.json, issuers.json) from the directory of
// SETTINGS_PATH, so the Docker /config/ and a local config/ both work. A missing file disables its feature; an
// invalid one is an error.
func loadCurated(cfg *config.Config) (*curated.Config, error) {
	return curated.Load(filepath.Dir(cfg.SettingsPath))
}

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

// exitRightsExcluded is the exit code of a successful publish whose export excluded rights profiles (Python `_build`
// exits 2 as well): the build is published, but a curated profile needs fixing.
const exitRightsExcluded = 2

// publishExit is the exit code of a publish that succeeded, given the code the command would otherwise return: a
// stronger failure code (nonzero) wins, then excluded rights profiles give 2. It prints the count to w.
func publishExit(w io.Writer, code int, res *publish.Result) int {
	n := res.Summary.RightsExcludedProfilesCount
	if n <= 0 {
		return code
	}
	_, _ = fmt.Fprintf(w, "  %d rights profiles excluded (see warnings and the run journal)\n", n)
	if code != 0 {
		return code
	}
	return exitRightsExcluded
}
