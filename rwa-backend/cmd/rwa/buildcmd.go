package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runBuild implements `rwa build [--run <id>]`: recompute the export from the stored snapshots, without network.
//
// It creates a NEW run (mode=rebuild, trigger=cli) and a build for it. A rebuild has no new observation, so it
// writes no history rows (publish.ModeRebuild): the charts would otherwise get a duplicate point. --run <id> names
// the run being rebuilt: it must exist and is recorded in the new run's run_key and log; the data are always the
// latest stored snapshot of each enabled underlying (the store has no point-in-time view).
func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	source := fs.Int64("run", 0, "id of the run being rebuilt (optional; recorded in the new run's key)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa build [--run <id>]")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateAPI()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 2
	}
	settingsJSON, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 1
	}
	defer pool.Close()
	log := newLogger(cfg, stderr)
	st := store.New(pool)

	suffix := ""
	if *source != 0 {
		src, err := st.GetRun(ctx, *source)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "rwa build: --run %d: %v\n", *source, err)
			return 1
		}
		suffix = "-of-" + strconv.FormatInt(src.ID, 10)
		log.Info("rebuilding", "source_run_id", src.ID, "source_run_key", src.RunKey)
	}
	runID, res, err := publish.New(pool, settings, assets, log).Rebuild(ctx, publish.RebuildParams{
		Trigger: "cli", KeySuffix: suffix, CollectorVersion: buildVersion(), SettingsJSON: settingsJSON})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa build:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, res.Line())
	_, _ = fmt.Fprintf(stdout, "  run %d, %d bytes, sha256 %s\n", runID, res.ExportBytes, res.ExportSHA256)
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintln(stdout, "  warning:", w)
	}
	return 0
}
