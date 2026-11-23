package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/collect"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/deals"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runDeals implements `rwa deals [--no-build]`, the counterpart of the Python `deals` command: fetch the cp0x deals
// the links config names, store them as the newest deals row, then rebuild the export (a rebuild run, no history)
// so the referrals are current. A failed fetch keeps the previous deals, prints the warning and exits 1 without a
// rebuild (Python prints the warning and rebuilds anyway; the previous deals stay cached, so only time-dependent results could
// differ, such as aged referrals dropping or reviews turning stale, and the next scheduled publish or `rwa build` applies those).
func runDeals(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("deals", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noBuild := fs.Bool("no-build", false, "store the deals only, do not rebuild the export")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa deals [--no-build]")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateAPI()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 2
	}
	assets, err := model.LoadAssets(cfg.AssetsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 2
	}
	settingsJSON, err := os.ReadFile(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 2
	}
	cur, err := loadCurated(cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 1
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals:", err)
		return 1
	}
	defer pool.Close()
	log, jh := newJournalLogger(cfg, stderr)
	jh.SetSink(store.New(pool))
	defer jh.Close()

	cache, warning, err := deals.New(nil).Refresh(ctx, cur.Links.ReferencedDeals())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "  warning %v\n", err)
		return 1
	}
	if err := collect.StoreDeals(ctx, store.New(pool), cache); err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals: store:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "deals: %d cached, fetched at %s\n", len(cache.Deals), cache.FetchedAt)
	if warning != "" {
		_, _ = fmt.Fprintf(stdout, "  warning %s\n", warning)
	}
	if *noBuild {
		return 0
	}
	_, res, err := publish.New(pool, settings, assets, log).WithCurated(cur).Rebuild(ctx, publish.RebuildParams{
		Trigger: "cli", KeySuffix: "-deals", CollectorVersion: buildVersion(), SettingsJSON: settingsJSON})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa deals: rebuild:", err)
		return 1
	}
	printPublish(stdout, res)
	return publishExit(stdout, 0, res)
}
