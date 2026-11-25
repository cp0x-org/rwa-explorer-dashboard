package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runPublish implements `rwa publish --run <id>`: publish an existing collection run (for example one made with
// `run-once --no-publish`, or one whose publish failed) WITH history rows. It refuses a run that already has a build,
// a rebuild run and a run that is still running or failed for another reason. Unlike `rwa build` it records a new
// observation: the history points of the run are written. Logos are not fetched; run-once or the collector do that.
func runPublish(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runID := fs.Int64("run", 0, "id of the collection run to publish (required)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *runID <= 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa publish --run <id>")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateAPI()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 2
	}
	cur, err := loadCurated(cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 1
	}
	defer pool.Close()
	log, jh := newJournalLogger(cfg, stderr)
	jh.SetSink(store.New(pool))
	defer jh.Close()
	st := store.New(pool)

	run, err := st.GetRun(ctx, *runID)
	if errors.Is(err, store.ErrNotFound) {
		_, _ = fmt.Fprintf(stderr, "rwa publish: run %d does not exist\n", *runID)
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 1
	}
	if err := checkPublishable(ctx, st, &run.Run); err != nil {
		_, _ = fmt.Fprintf(stderr, "rwa publish: run %d: %v\n", run.ID, err)
		return 1
	}
	defer runlog.CloseRun(log, run.ID) // the last event of the run; runs before jh.Close
	res, err := publish.New(pool, settings, assets, log).WithCurated(cur).PublishWith(ctx, run.ID, publish.Options{WriteHistory: true})
	if err != nil {
		if merr := st.MarkRunPublishFailed(ctx, run.ID, err); merr != nil {
			log.Error("recording the publish failure on the run failed", "run_id", run.ID, "err", merr)
		}
		_, _ = fmt.Fprintln(stderr, "rwa publish:", err)
		return 1
	}
	if err := st.ClearRunPublishFailed(ctx, run.ID); err != nil {
		log.Error("restoring the run status failed", "run_id", run.ID, "err", err)
	}
	printPublish(stdout, res)
	_, _ = fmt.Fprintf(stdout, "  run %d, %d bytes, sha256 %s\n", run.ID, res.ExportBytes, res.ExportSHA256)
	return publishExit(stdout, 0, res)
}

// checkPublishable reports why a run cannot be published with history, or nil.
func checkPublishable(ctx context.Context, st *store.Store, run *store.Run) error {
	switch {
	case run.Mode == publish.ModeRebuild:
		return errors.New("is a rebuild: it has no observation to publish (use `rwa build`)")
	case run.Status == "running":
		return errors.New("is still running")
	case run.Status == "failed" && (run.Error == nil || !strings.HasPrefix(*run.Error, "publish:")):
		return errors.New("failed during collection and has no complete data to publish")
	}
	has, err := st.RunHasBuild(ctx, run.ID)
	if err != nil {
		return err
	}
	if has {
		return errors.New("already has a build (use `rwa build` to recompute the export)")
	}
	return nil
}
