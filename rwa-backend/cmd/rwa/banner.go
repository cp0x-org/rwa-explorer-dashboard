package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/banner"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/deals"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
)

// The startup banner of each long-running role (and of `migrate up`): what the process is, where it listens,
// which database and tables it uses, which external services it calls and which config files it read. Secrets are
// never printed: the database password is masked, API keys and the admin token are shown as set or not set.

// Tables each role touches; kept next to the banner so the operator does not have to read the store package.
const (
	apiReads  = "build_exports (current export), build_changes, runs, run_underlyings, run_events,\nhistory_* (charts), catalog_logos (/logos)"
	apiWrites = "run_requests (admin API: manual run requests for the collector)"

	collectorReads  = "run_requests (manual runs, polled every 30 s), raw_snapshots (input of every build)"
	collectorWrites = "runs, run_underlyings, run_events (journal), raw_snapshots, raw_cp0x_deals,\ncatalog_* (entities, logos), history_* (one row per entity and run),\nbuild_exports, build_changes"
)

func printBanner(w io.Writer, cfg *config.Config, role, subtitle string, sections ...banner.Section) {
	if !cfg.StartupBanner {
		return
	}
	head := banner.Section{Title: "service", Rows: []banner.Row{
		banner.Rf("version", "%s (%s, %s/%s)", buildVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH),
		banner.Rf("host", "%s, pid %d", hostname(), os.Getpid()),
		banner.R("started", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")),
		banner.Rf("logs", "%s to stderr, level %s", cfg.LogFormat, cfg.LogLevel),
	}}
	banner.Render(w, "RWA Explorer · "+role, subtitle, append([]banner.Section{head}, sections...))
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// databaseSection describes DATABASE_URL (password masked) and the schema state. A failed status lookup is shown,
// not fatal: the caller's own checks report the error.
func databaseSection(ctx context.Context, cfg *config.Config, reads, writes string) banner.Section {
	rows := []banner.Row{banner.R("url", cfg.MaskedDatabaseURL())}
	if u, err := url.Parse(cfg.DatabaseURL); err == nil && u.Host != "" {
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		rows = append(rows, banner.Rf("server", "%s, database %s, user %s, sslmode %s",
			u.Host, strings.TrimPrefix(u.Path, "/"), user, orDefault(u.Query().Get("sslmode"), "prefer")))
	}
	rows = append(rows, banner.R("schema", schemaState(ctx, cfg.DatabaseURL)))
	if reads != "" {
		rows = append(rows, banner.R("reads", reads))
	}
	if writes != "" {
		rows = append(rows, banner.R("writes", writes))
	}
	return banner.Section{Title: "database (PostgreSQL)", Rows: rows}
}

func schemaState(ctx context.Context, dbURL string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := db.Status(ctx, dbURL)
	if err != nil {
		return "unknown (" + firstLine(err.Error()) + ")"
	}
	var applied, latest int64
	pending := 0
	for _, m := range st {
		latest = max(latest, m.Version)
		if m.Applied {
			applied = max(applied, m.Version)
		} else {
			pending++
		}
	}
	if pending == 0 {
		return fmt.Sprintf("migration %04d of %04d, up to date", applied, latest)
	}
	return fmt.Sprintf("migration %04d of %04d, %d pending", applied, latest, pending)
}

func apiBanner(ctx context.Context, w io.Writer, cfg *config.Config) {
	listen := []banner.Row{banner.Rf("http", "%s (HTTP_ADDR)", cfg.HTTPAddr)}
	if cfg.PublicURL != "" {
		listen = append(listen, banner.Rf("public", "%s  → status %s/api/v1/status", cfg.PublicURL, cfg.PublicURL))
	}
	listen = append(listen,
		banner.R("endpoints", "/api/v1/{export,status,summary,underlyings,tokens,issuers,opportunities}\n/api/v1/{runs,history}/…, /api/v1/openapi.yaml, /logos/{kind}/{file}, /healthz, /readyz"),
		banner.R("admin", onOff(cfg.AdminToken != "", "POST /api/v1/admin/runs, Bearer ADMIN_TOKEN (set)", "disabled: ADMIN_TOKEN is empty, /api/v1/admin/* answers 404")),
		banner.R("cors", onOff(len(cfg.CORSOrigins) > 0, strings.Join(cfg.CORSOrigins, ", "), "off: same origin only (CORS_ORIGINS empty)")),
		banner.R("cache", "the current build is re-checked every 15 s; /readyz is 503 until a build exists"),
	)
	printBanner(w, cfg, "api", "HTTP API over PostgreSQL: the export, catalog, history and runs for the site",
		banner.Section{Title: "listens", Rows: listen},
		databaseSection(ctx, cfg, apiReads, apiWrites),
	)
}

func collectorBanner(ctx context.Context, w io.Writer, cfg *config.Config, assets []model.Asset) {
	schedule := []banner.Row{}
	if cfg.CollectInterval > 0 {
		schedule = append(schedule, banner.Rf("every", "%s from the collector start (COLLECT_INTERVAL)", shortDuration(cfg.CollectInterval)))
	} else {
		schedule = append(schedule, banner.Rf("cron", "%q UTC (COLLECT_CRON)", cfg.CollectCron))
	}
	schedule = append(schedule,
		banner.R("on start", onOff(cfg.CollectOnStart, "collect at once when the last successful run is stale (COLLECT_ON_START)", "no run on start (COLLECT_ON_START=false)")),
		banner.Rf("timeout", "%s per run (RUN_TIMEOUT)", shortDuration(cfg.RunTimeout)),
		banner.Rf("retention", "raw_snapshots %d days, run_events %d days (the latest are always kept)", cfg.SnapshotRetentionDays, cfg.RunEventsRetentionDays),
		banner.Rf("journal", "records with a run id at level %s and above go to run_events (/updates)", cfg.RunEventsLevel),
		banner.R("migrate", onOff(cfg.MigrateOnStart, "applies pending migrations on start (MIGRATE_ON_START)", "only checks the schema (MIGRATE_ON_START=false)")),
	)
	if cfg.RawCaptureDir != "" {
		schedule = append(schedule, banner.Rf("raw dir", "raw API responses saved to %s (RAW_CAPTURE_DIR)", cfg.RawCaptureDir))
	}

	sources := banner.Section{Title: "connects to", Rows: []banner.Row{
		banner.Rf("CMC", "%s  key %s (CMC_API_KEY)", cmc.BaseURL, setOrNot(cfg.CMCAPIKey)),
		banner.Rf("Portals", "%s  key %s (PORTALS_API_KEY)", portals.BaseURL, setOrNot(cfg.PortalsAPIKey)),
		banner.Rf("cp0x deals", "%s  (once per run, for referral links)", deals.URL),
		banner.R("logos", "CMC logo URLs, then Google favicon service, then the site's <link rel=icon>"),
	}}

	enabled := 0
	for _, a := range assets {
		if a.Enabled {
			enabled++
		}
	}
	dir := filepath.Dir(cfg.SettingsPath)
	files := banner.Section{Title: "config files", Rows: []banner.Row{
		banner.Rf("assets", "%s: %d underlyings, %d enabled (ASSETS_PATH)", cfg.AssetsPath, len(assets), enabled),
		banner.Rf("settings", "%s (SETTINGS_PATH)", cfg.SettingsPath),
		banner.Rf("curated", "%s: %s", dir, curatedFiles(dir)),
	}}

	printBanner(w, cfg, "collector", "scheduled collector: CoinMarketCap + Portals → raw_snapshots → ratings → build_exports + history",
		banner.Section{Title: "schedule", Rows: schedule},
		sources,
		files,
		databaseSection(ctx, cfg, collectorReads, collectorWrites),
	)
}

func migrateBanner(ctx context.Context, w io.Writer, cfg *config.Config) {
	printBanner(w, cfg, "migrate", "applies the embedded SQL migrations (internal/db/migrations) and exits",
		databaseSection(ctx, cfg, "", "the schema of every table (goose_db_version tracks the applied migrations)"))
}

// curatedFiles lists the curated config files next to settings.json and whether each was found.
func curatedFiles(dir string) string {
	var parts []string
	for _, f := range []string{curated.LinksFile, curated.RightsFile, curated.IssuersFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			parts = append(parts, f)
		} else {
			parts = append(parts, f+" (missing: feature off)")
		}
	}
	return strings.Join(parts, ", ")
}

// shortDuration prints 24h instead of 24h0m0s and 90m as 1h30m.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

func onOff(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func setOrNot(s string) string { return onOff(s != "", "set", "NOT SET") }

func orDefault(s, def string) string { return onOff(s != "", s, def) }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
