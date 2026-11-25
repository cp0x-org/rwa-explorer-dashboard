// Command rwa is the single binary of RWA Explorer 2.0: collector, API and maintenance tasks.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
)

// version is set at build time: -ldflags "-X main.version=...".
var version = "dev"

type command struct {
	name string
	desc string
}

// stubs are subcommands that are not implemented yet.
var stubs = []command{}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: rwa <command> [flags]")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "api", "serve the HTTP API")
	for _, c := range stubs {
		_, _ = fmt.Fprintf(w, "  %-10s %s\n", c.name, c.desc)
	}
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "collector", "run the scheduled data collector (cron/interval, run requests)")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "run-once", "one collection job: collect, publish, logos: --mode refresh|add --symbols NVDA[,X] --raw-dir DIR --no-publish")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "deals", "fetch the cp0x deals now and store them, then rebuild the export: [--no-build]")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "build", "recompute the export from stored snapshots, no history: [--run <id>]")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "publish", "publish an existing collection run with history (no build yet): --run <id>")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "migrate", "database migrations: up | status | down-to <version> [--yes]")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "probe", "dev only: live check of a source client: --provider cmc|portals --symbols NVDA[,X]")
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "version", "print the version")
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	if err := config.LoadDotEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "rwa: load .env:", err)
		os.Exit(2)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "version", "--version":
		_, _ = fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if args[0] == "migrate" {
		return runMigrate(args[1:], stdout, stderr)
	}
	if args[0] == "probe" {
		return runProbe(args[1:], stdout, stderr)
	}
	switch args[0] {
	case "api":
		return runAPI(args[1:], stdout, stderr)
	case "collector":
		return runCollector(args[1:], stdout, stderr)
	case "run-once":
		return runOnce(args[1:], stdout, stderr)
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "deals":
		return runDeals(args[1:], stdout, stderr)
	case "publish":
		return runPublish(args[1:], stdout, stderr)
	}
	for _, c := range stubs {
		if c.name == args[0] {
			_, _ = fmt.Fprintf(stderr, "rwa %s: not implemented\n", c.name)
			return 2
		}
	}
	_, _ = fmt.Fprintf(stderr, "rwa: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

// runMigrate implements `rwa migrate up|status|down-to <version>`. Only DATABASE_URL is required.
func runMigrate(args []string, stdout, stderr io.Writer) int {
	const usageLine = "usage: rwa migrate up | status | down-to <version> [--yes]"
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, usageLine)
		return 2
	}
	var version int64
	var yes bool
	if args[0] == "down-to" {
		rest := make([]string, 0, len(args))
		for _, a := range args {
			if a == "--yes" || a == "-y" {
				yes = true
			} else {
				rest = append(rest, a)
			}
		}
		args = rest
	}
	switch args[0] {
	case "up", "status":
		if len(args) != 1 {
			_, _ = fmt.Fprintln(stderr, usageLine)
			return 2
		}
	case "down-to":
		if len(args) != 2 {
			_, _ = fmt.Fprintln(stderr, usageLine)
			return 2
		}
		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || v < 0 {
			_, _ = fmt.Fprintf(stderr, "rwa migrate: invalid version %q\n", args[1])
			return 2
		}
		version = v
	default:
		_, _ = fmt.Fprintf(stderr, "rwa migrate: unknown action %q\n%s\n", args[0], usageLine)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa migrate:", err)
		return 2
	}
	if cfg.DatabaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "rwa migrate: DATABASE_URL is required")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "up":
		migrateBanner(ctx, stderr, cfg)
		err = db.Up(ctx, cfg.DatabaseURL)
		if err == nil {
			var v int64
			if v, err = db.Version(ctx, cfg.DatabaseURL); err == nil {
				_, _ = fmt.Fprintf(stdout, "migrations applied, current version %d\n", v)
			}
		}
	case "down-to":
		var cur int64
		if cur, err = db.Version(ctx, cfg.DatabaseURL); err != nil {
			break
		}
		if version >= cur {
			_, _ = fmt.Fprintf(stdout, "already at version %d, nothing to roll back\n", cur)
			break
		}
		if !yes {
			_, _ = fmt.Fprintf(stderr, "rwa migrate: down-to %d would roll back from version %d and may destroy data; re-run with --yes\n", version, cur)
			return 2
		}
		err = db.DownTo(ctx, cfg.DatabaseURL, version)
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "rolled back to version %d\n", version)
		}
	case "status":
		var st []db.MigrationStatus
		if st, err = db.Status(ctx, cfg.DatabaseURL); err == nil {
			for _, m := range st {
				state := "pending"
				if m.Applied {
					state = "applied " + m.AppliedAt.UTC().Format("2006-01-02T15:04:05Z")
				}
				_, _ = fmt.Fprintf(stdout, "%04d  %-24s %s\n", m.Version, m.Source, state)
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa migrate:", err)
		return 1
	}
	return 0
}
