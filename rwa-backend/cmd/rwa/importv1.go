package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/importer"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// runImportV1 implements `rwa import-v1 --data-dir <path>`: it imports the 1.x pipeline data as one run and
// publishes a build. A second import of the same 1.x run is a no-op (exit 0).
func runImportV1(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import-v1", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", "", "1.x data folder (rwa-data-pipeline/data)")
	force := fs.Bool("force", false, "import even when the database already has collection runs")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dataDir == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa import-v1 --data-dir <path> [--force]")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateAPI()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 2
	}
	settingsJSON, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 1
	}
	defer pool.Close()

	log := newLogger(cfg, stderr)
	res, err := importer.New(pool, assets, settings, settingsJSON, buildVersion(), log).WithForce(*force).Import(ctx, *dataDir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa import-v1:", err)
		return 1
	}
	if res.Skipped {
		_, _ = fmt.Fprintf(stdout, "import skipped: run %s already exists (run id %d)\n", res.RunKey, res.RunID)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "imported run %s (run id %d): %d snapshots, %d logos\n", res.RunKey, res.RunID, res.Snapshots, res.Logos)
	_, _ = fmt.Fprintln(stdout, res.Build.Line())
	return 0
}
