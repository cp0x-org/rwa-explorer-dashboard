package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/collect"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/logos"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/scheduler"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runCollector implements `rwa collector`: the long-running scheduler (cron or interval, on-start run, manual
// run requests). It migrates the database first unless MIGRATE_ON_START=false.
func runCollector(args []string, _, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa collector")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateCollector()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa collector:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa collector:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa collector:", err)
		return 2
	}
	settingsJSON, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa collector:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := newLogger(cfg, stderr)

	if cfg.MigrateOnStart {
		if err := db.Up(ctx, cfg.DatabaseURL); err != nil {
			log.Error("migrate on start failed", "err", err)
			return 1
		}
	} else if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		log.Error("database is not migrated", "err", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "err", err)
		return 1
	}
	defer closePool(pool, log)

	if _, err := collect.AbandonRunningRuns(ctx, pool, log); err != nil {
		log.Error("abandoned-run cleanup failed", "err", err)
		return 1
	}

	st := store.New(pool)
	pub := publish.New(pool, settings, assets, log)
	version := buildVersion()
	collect.Version = version
	runner := collect.NewRunner(pool, *cfg, settings, assets, log)
	fetcher := logos.NewFetcher(pool, log, nil)
	deps := scheduler.Deps{
		// One collection run: snapshots into the database; publishing is the next step of the job.
		Collect: func(ctx context.Context, mode string, symbols []string, trigger string) (scheduler.CollectResult, error) {
			res, err := runner.Run(ctx, collect.Options{Mode: mode, Symbols: symbols, Trigger: trigger})
			if res == nil {
				return scheduler.CollectResult{}, err
			}
			// The run id travels with an error too (interrupted run, FinishRun failure): the request is attached to it.
			if res.Err != nil {
				log.Error("collection run finished with a fatal error", "run_id", res.RunID, "err", res.Err)
			}
			return scheduler.CollectResult{RunID: res.RunID, Status: res.Status, NothingToDo: res.NothingToDo}, err
		},
		Publish: func(ctx context.Context, runID int64) error {
			run, err := st.GetRun(ctx, runID)
			if err != nil {
				return err
			}
			_, err = pub.Publish(ctx, runID, run.Mode)
			return err
		},
		MarkPublishFailed: st.MarkRunPublishFailed,
		Rebuild: func(ctx context.Context) (int64, error) {
			runID, _, err := pub.Rebuild(ctx, publish.RebuildParams{Trigger: "manual", CollectorVersion: version, SettingsJSON: settingsJSON})
			return runID, err
		},
		// Logos run after the publish (the dimension tables must know the new entities first). When new logos
		// arrived, a rebuild (no history rows) puts their paths into the current export right away, as 1.x did by
		// fetching logos before build.
		Logos: func(ctx context.Context, trigger string) error {
			rep, err := fetcher.FetchMissing(ctx)
			if err != nil {
				return err
			}
			if len(rep.Downloaded) == 0 {
				return nil
			}
			log.Info("new logos downloaded, rebuilding the export", "count", len(rep.Downloaded))
			_, _, err = pub.Rebuild(ctx, publish.RebuildParams{Trigger: trigger, CollectorVersion: version, SettingsJSON: settingsJSON})
			return err
		},
		Retention: func(ctx context.Context, cutoff time.Time) (int64, error) {
			return st.DeleteSnapshotsOlderThan(ctx, cutoff)
		},
		TakeRequest:    st.TakeNextRunRequest,
		AttachRun:      st.AttachRunToRequest,
		ReleaseRequest: st.ReleaseRunRequest,
		FinishRequest:  st.FinishRunRequest,
		LastRun: func(ctx context.Context) (time.Time, bool, error) {
			t, err := st.LastSuccessfulRunStart(ctx)
			if errors.Is(err, store.ErrNotFound) {
				return time.Time{}, false, nil
			}
			return t, err == nil, err
		},
	}
	sched, err := scheduler.New(scheduler.Config{
		Cron: cfg.CollectCron, Interval: cfg.CollectInterval, OnStart: cfg.CollectOnStart, RetentionDays: cfg.SnapshotRetentionDays,
		RunTimeout: cfg.RunTimeout,
	}, deps, log)
	if err != nil {
		log.Error("scheduler configuration invalid", "err", err)
		return 2
	}
	log.Info("collector starting", "version", version, "config", cfg.String())
	if err := sched.Run(ctx); err != nil {
		log.Error("collector stopped with an error", "err", err)
		return 1
	}
	return 0
}

// closePool closes the pool in the background and waits at most 5 s: pool.Close blocks until every connection is
// released, and a job that ignored the shutdown deadline would hold one forever. The process exits right after.
func closePool(pool *pgxpool.Pool, log *slog.Logger) {
	done := make(chan struct{})
	go func() { pool.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Warn("closing the database pool timed out, exiting anyway")
	}
}
