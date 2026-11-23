package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/collect"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/logos"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runOnce implements `rwa run-once [--mode refresh|add] [--symbols NVDA,TSLA] [--raw-dir DIR] [--no-publish]`: one
// scheduler job without the scheduler — collection run, then publish (export + history), then missing logos (with a
// rebuild when new logos arrived). --no-publish stops after the snapshots. Exit code: 0 succeeded, 2 partial (some
// underlyings failed, a published export excluded rights profiles, or flag/config error), 1 failed (fatal error, nothing completed, publish error).
func runOnce(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run-once", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", collect.ModeRefresh, "refresh (all selected) or add (only underlyings without a snapshot)")
	symbols := fs.String("symbols", "", "comma-separated tickers; default all enabled")
	rawDir := fs.String("raw-dir", "", "write raw API responses to <dir>/<run_key>/ (default $RAW_CAPTURE_DIR)")
	noPublish := fs.Bool("no-publish", false, "stop after the snapshots: no export, history or logos (publish later with `rwa publish --run <id>`)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (*mode != collect.ModeRefresh && *mode != collect.ModeAdd) {
		_, _ = fmt.Fprintln(stderr, "usage: rwa run-once [--mode refresh|add] [--symbols NVDA,TSLA] [--raw-dir DIR] [--no-publish]")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateCollector()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 2
	}
	if *rawDir != "" {
		cfg.RawCaptureDir = *rawDir
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 2
	}
	cur, err := loadCurated(cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 2
	}
	var syms []string
	for _, s := range strings.Split(*symbols, ",") {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			syms = append(syms, s)
		}
	}
	if _, err := collect.SelectAssets(assets, syms); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancelTimeout := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancelTimeout()

	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 1
	}
	defer pool.Close()

	collect.Version = buildVersion()
	log, jh := newJournalLogger(cfg, stderr)
	jh.SetSink(store.New(pool))
	defer jh.Close()
	var journalRun int64
	defer func() { runlog.CloseRun(log, journalRun) }() // the last event of the run; runs before jh.Close
	res, err := collect.NewRunner(pool, *cfg, settings, assets, log).WithCurated(cur).Run(ctx, collect.Options{Mode: *mode, Symbols: syms, Trigger: "cli"})
	if res != nil {
		journalRun = res.RunID
	}
	if res != nil && res.RunID != 0 {
		printRunReport(ctx, stdout, store.New(pool), res)
	}
	switch {
	case errors.Is(err, collect.ErrRunInProgress):
		_, _ = fmt.Fprintln(stderr, "rwa run-once: another collector run is in progress")
		return 1
	case errors.Is(err, context.DeadlineExceeded):
		_, _ = fmt.Fprintf(stderr, "rwa run-once: run timed out after %s (RUN_TIMEOUT)\n", cfg.RunTimeout)
		return 1
	case err != nil:
		_, _ = fmt.Fprintln(stderr, "rwa run-once:", err)
		return 1
	case res.NothingToDo:
		_, _ = fmt.Fprintln(stdout, "nothing to add: every selected underlying already has a snapshot")
		return 0
	}
	if res.Status != collect.RunSucceeded {
		_, _ = fmt.Fprintf(stdout, "run %s: %s\n", res.RunKey, res.Status)
	}
	code := 1
	switch res.Status {
	case collect.RunSucceeded:
		code = 0
	case collect.RunPartial:
		code = 2
	}
	if *noPublish || res.Status == collect.RunFailed {
		if !*noPublish {
			_, _ = fmt.Fprintln(stdout, "run failed: export not rebuilt")
		}
		return code
	}
	st := store.New(pool)
	pub := publish.New(pool, settings, assets, log).WithCurated(cur)
	pubRes, err := pub.Publish(ctx, res.RunID, *mode)
	if err != nil {
		if merr := st.MarkRunPublishFailed(ctx, res.RunID, err); merr != nil {
			log.Error("recording the publish failure on the run failed", "run_id", res.RunID, "step", "publish", "err", merr)
		}
		_, _ = fmt.Fprintln(stderr, "rwa run-once: publish:", err)
		return 1
	}
	printPublish(stdout, pubRes)
	// The export is published; a logos problem does not change the outcome of the collection.
	if err := logosAndRebuild(ctx, stdout, pool, pub, cfg, res.RunID, log); err != nil {
		log.Error("logos step failed", "run_id", res.RunID, "step", "logos", "err", err)
		_, _ = fmt.Fprintln(stderr, "rwa run-once: logos:", err)
	}
	return publishExit(stdout, code, pubRes)
}

func printPublish(w io.Writer, res *publish.Result) {
	_, _ = fmt.Fprintln(w, res.Line())
	for _, warning := range res.Warnings {
		_, _ = fmt.Fprintf(w, "  warning: %s\n", warning)
	}
}

// logosAndRebuild downloads missing logos and, when new logos arrived, rebuilds the export (no history) so their
// paths are served.
func logosAndRebuild(ctx context.Context, w io.Writer, pool *pgxpool.Pool, pub *publish.Publisher, cfg *config.Config, runID int64, log *slog.Logger) error {
	rep, err := logos.NewFetcher(pool, log.With("run_id", runID, "step", "logos"), nil).FetchMissing(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "logos: %d downloaded, %d failed, %d without source\n", len(rep.Downloaded), len(rep.Failed), len(rep.MissingSource))
	if len(rep.Downloaded) == 0 {
		return nil
	}
	settingsJSON, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		return err
	}
	_, res, err := pub.Rebuild(ctx, publish.RebuildParams{Trigger: "cli", KeySuffix: fmt.Sprintf("-logos-of-%d", runID), CollectorVersion: buildVersion(), SettingsJSON: settingsJSON})
	if err != nil {
		return fmt.Errorf("rebuild after logos: %w", err)
	}
	_, _ = fmt.Fprintln(w, res.Line())
	return nil
}

// printRunReport prints the 1.x-style summary of a finished run.
func printRunReport(ctx context.Context, w io.Writer, st *store.Store, res *collect.Result) {
	_, _ = fmt.Fprintf(w, "run %s (id %d): %s\n", res.RunKey, res.RunID, res.Status)
	_, _ = fmt.Fprintf(w, "  completed: %d, failed: %d, warnings: %d\n", res.Completed, res.Failed, len(res.Warnings))
	_, _ = fmt.Fprintf(w, "  requests: cmc=%d portals=%d, cmc credits=%d\n", res.CMCRequests, res.PortalsRequests, res.CMCCredits)
	if res.Err != nil {
		_, _ = fmt.Fprintf(w, "  fatal: %v\n", res.Err)
	}
	for _, warning := range res.Warnings {
		_, _ = fmt.Fprintf(w, "  warning: %s\n", warning)
	}
	detail, err := st.GetRun(context.WithoutCancel(ctx), res.RunID)
	if err != nil {
		return
	}
	for _, u := range detail.Underlyings {
		if u.Status == "failed" && u.Error != nil {
			_, _ = fmt.Fprintf(w, "  failed %s: %s\n", u.UnderlyingID, *u.Error)
		}
	}
}
