package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
)

// runProbe implements the dev-only `rwa probe --provider cmc|portals --symbols NVDA[,X]`: a few live
// calls to check keys and the source clients. It never prints keys.
func runProbe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "cmc or portals")
	symbols := fs.String("symbols", "", "comma-separated tickers, e.g. NVDA,AAPL")
	assetType := fs.String("type", "stock", "asset type used to pick the RWA record (stock|etf)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var tickers []string
	for _, s := range strings.Split(*symbols, ",") {
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			tickers = append(tickers, s)
		}
	}
	if (*provider != "cmc" && *provider != "portals") || len(tickers) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa probe --provider cmc|portals --symbols NVDA[,AAPL] [--type stock|etf]")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateSources(*provider)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa probe:", err)
		return 2
	}
	settings, err := model.LoadSettings(cfg.SettingsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa probe:", err)
		return 2
	}
	// The collector must pass a per-run subfolder as the raw capture dir: file names restart at 0001 for
	// every client, so a shared directory would overwrite earlier captures.
	rawDir := ""
	if cfg.RawCaptureDir != "" {
		rawDir = filepath.Join(cfg.RawCaptureDir, "probe-"+time.Now().UTC().Format("20060102T150405Z"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hook := func(p, path string, params url.Values, status int, d time.Duration) {
		_, _ = fmt.Fprintf(stderr, "  %s GET %s -> %d (%s)\n", p, path, status, d.Round(time.Millisecond))
	}
	switch *provider {
	case "cmc":
		api := cmc.NewHTTPClient(cfg.CMCAPIKey, settings, rawDir, hook)
		err = probeCMC(ctx, stdout, cmc.New(api), api, settings, tickers, *assetType)
	default:
		api := portals.NewHTTPClient(cfg.PortalsAPIKey, settings, rawDir, hook)
		err = probePortals(ctx, stdout, portals.New(api), api, settings, tickers)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa probe:", err)
		return 1
	}
	return 0
}

func str(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func probeCMC(ctx context.Context, w io.Writer, c *cmc.Client, api *httpx.Client, s *model.Settings, tickers []string, assetType string) error {
	quotes, err := c.FetchRWAQuotes(ctx, tickers)
	if err != nil {
		return err
	}
	var cryptoIDs []int
	for _, t := range tickers {
		asset := model.Asset{Ticker: t, AssetType: assetType}
		picked, reason := cmc.PickRWAAsset(quotes[t], asset, nil)
		if picked == nil {
			_, _ = fmt.Fprintf(w, "%s: not resolved (%s), %d candidate(s)\n", t, reason, len(quotes[t]))
			continue
		}
		rwaID := "-"
		if picked.RwaID != nil {
			rwaID = fmt.Sprint(*picked.RwaID)
		}
		_, _ = fmt.Fprintf(w, "%s: rwa_id=%s %q tokens=%d\n", t, rwaID, str(picked.Name), len(picked.Tokens))
		for _, tok := range picked.Tokens {
			id := "-"
			if tok.CryptoID != nil {
				id = fmt.Sprint(*tok.CryptoID)
				cryptoIDs = append(cryptoIDs, int(*tok.CryptoID))
			}
			_, _ = fmt.Fprintf(w, "  %-12s issuer=%-28s crypto_id=%s\n", str(tok.Symbol), str(tok.IssuerName), id)
		}
	}
	if len(cryptoIDs) > 0 {
		info, invalid, err := c.FetchCryptoInfo(ctx, cryptoIDs)
		if err != nil {
			return err
		}
		deployments := 0
		for _, in := range info {
			deployments += len(compute.ParseDeployments(&in.CMCInfo, s.ChainMapping))
		}
		_, _ = fmt.Fprintf(w, "crypto info: %d entries, %d invalid ids %v, %d deployments\n", len(info), len(invalid), invalid, deployments)
	}
	_, _ = fmt.Fprintf(w, "requests=%d credits=%d\n", api.Requests(), api.Credits())
	return nil
}

func probePortals(ctx context.Context, w io.Writer, c *portals.Client, api *httpx.Client, s *model.Settings, tickers []string) error {
	nets, err := c.FetchNetworks(ctx)
	if err != nil {
		return err
	}
	names, err := c.FetchPlatformNames(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "networks=%d platforms=%d\n", len(nets), len(names))
	for _, term := range portals.MinimalTerms(tickers) {
		_, meta, err := c.Search(ctx, term, s.PortalsPageLimit)
		if err != nil {
			return err
		}
		total := "-"
		if meta.TotalItems != nil {
			total = fmt.Sprint(*meta.TotalItems)
		}
		_, _ = fmt.Fprintf(w, "search %q: total=%s received=%d pages=%d complete=%t count_mismatch=%t\n",
			term, total, meta.Received, meta.Pages, meta.Complete, meta.CountMismatch)
	}
	_, _ = fmt.Fprintf(w, "requests=%d\n", api.Requests())
	return nil
}
