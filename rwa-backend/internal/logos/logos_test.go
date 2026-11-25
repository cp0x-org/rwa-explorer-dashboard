package logos

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\nfake")

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func imageFile(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

func TestDownload(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/logo.png":
			assert.Contains(t, r.Header.Get("User-Agent"), "Mozilla/5.0")
			imageFile(w, "image/png; charset=binary", pngBytes)
		case "/logo.jpeg":
			imageFile(w, "image/jpeg", pngBytes)
		case "/vector":
			imageFile(w, "image/svg+xml", []byte("<svg/>"))
		case "/no-ext-type.avif":
			imageFile(w, "image/avif", pngBytes) // not in the table: extension from the URL path, else png
		case "/odd":
			imageFile(w, "image/avif", pngBytes)
		case "/big.png":
			imageFile(w, "image/png", bytes.Repeat([]byte{1}, MaxBytes+1))
		case "/exact.png":
			imageFile(w, "image/png", bytes.Repeat([]byte{1}, MaxBytes))
		case "/empty.png":
			imageFile(w, "image/png", nil)
		case "/page":
			imageFile(w, "text/html", []byte("<html/>"))
		case "/missing":
			http.NotFound(w, r)
		}
	})
	f := newFetcher(nil, discard(), srv.Client())
	ctx := context.Background()

	img, err := f.download(ctx, srv.URL+"/logo.png")
	require.NoError(t, err)
	assert.Equal(t, "png", img.ext)
	assert.Equal(t, "image/png", img.contentType)
	assert.Equal(t, pngBytes, img.data)

	img, err = f.download(ctx, srv.URL+"/logo.jpeg")
	require.NoError(t, err)
	assert.Equal(t, "jpg", img.ext)

	img, err = f.download(ctx, srv.URL+"/vector")
	require.NoError(t, err)
	assert.Equal(t, "svg", img.ext)

	img, err = f.download(ctx, srv.URL+"/no-ext-type.avif")
	require.NoError(t, err)
	assert.Equal(t, "png", img.ext, "unknown extension falls back to png")

	img, err = f.download(ctx, srv.URL+"/exact.png")
	require.NoError(t, err)
	assert.Len(t, img.data, MaxBytes, "exactly the limit is accepted")

	for _, p := range []string{"/big.png", "/empty.png", "/page", "/missing"} {
		_, err := f.download(ctx, srv.URL+p)
		assert.Error(t, err, p)
	}
}

func TestIconFromHTML(t *testing.T) {
	base, _ := url.Parse("https://example.com/en/")
	for name, tc := range map[string]struct{ page, want string }{
		"largest size wins": {
			`<head><link rel="icon" sizes="16x16" href="/16.png"><link rel='icon' sizes='192x192' href='/192.png'><link rel="icon" sizes="32x32" href="/32.png"></head>`,
			"https://example.com/192.png",
		},
		"any is 512": {
			`<link rel="icon" sizes="256x256" href="a.png"><link rel="icon" sizes="any" href="icon.svg">`,
			"https://example.com/en/icon.svg",
		},
		"declared beats undeclared, touch icon beats plain": {
			`<link rel="shortcut icon" href="/plain.ico"><link rel="apple-touch-icon" href="/touch.png"><link rel="icon" sizes="16x16" href="/small.png">`,
			"https://example.com/touch.png",
		},
		"undeclared sizes: first wins on a tie": {
			`<link rel="icon" href="/first.png"><link rel="icon" href="/second.png">`,
			"https://example.com/first.png",
		},
		"absolute href":      {`<LINK REL="ICON" HREF="https://cdn.example.net/i.png">`, "https://cdn.example.net/i.png"},
		"unquoted":           {`<link rel=icon href=/u.png>`, "https://example.com/u.png"},
		"stylesheet ignored": {`<link rel="stylesheet" href="/s.css">`, "https://example.com/favicon.ico"},
		"nothing declared":   {`<html></html>`, "https://example.com/favicon.ico"},
		"no href":            {`<link rel="icon">`, "https://example.com/favicon.ico"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, IconFromHTML(tc.page, base))
		})
	}
}

func TestFetchOrder(t *testing.T) {
	var hits []string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		switch r.URL.Path {
		case "/cmc-broken":
			http.Error(w, "gone", http.StatusGone)
		case "/favicons":
			assert.Equal(t, "127.0.0.1", r.URL.Query().Get("domain"))
			http.Error(w, "nope", http.StatusNotFound)
		case "/site":
			_, _ = fmt.Fprint(w, `<link rel="icon" sizes="64x64" href="/declared.png">`)
		case "/declared.png":
			imageFile(w, "image/png", pngBytes)
		}
	})
	f := newFetcher(nil, discard(), srv.Client())
	f.faviconFmt = srv.URL + "/favicons?domain=%s"

	img, source := f.fetch(context.Background(), srv.URL+"/cmc-broken", srv.URL+"/site")
	require.NotNil(t, img)
	assert.Equal(t, srv.URL+"/declared.png", source)
	assert.Equal(t, []string{"/cmc-broken", "/favicons", "/site", "/declared.png"}, hits)
}

func TestFetchNothingWorks(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusInternalServerError) })
	f := newFetcher(nil, discard(), srv.Client())
	f.faviconFmt = srv.URL + "/f?domain=%s"
	img, _ := f.fetch(context.Background(), srv.URL+"/a", srv.URL+"/b")
	assert.Nil(t, img)
}

func ptr[T any](v T) *T { return &v }

func TestFetchMissingIntegration(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	st := store.New(pool)

	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token.png":
			imageFile(w, "image/png", pngBytes)
		case "/oversize.png":
			imageFile(w, "image/png", bytes.Repeat([]byte{1}, MaxBytes+1))
		case "/html-logo":
			imageFile(w, "text/html", []byte("<html/>"))
		case "/favicons":
			if r.URL.Query().Get("domain") == "issuer-a.test" {
				imageFile(w, "image/x-icon", []byte("ICO"))
				return
			}
			http.NotFound(w, r)
		case "/issuer-b":
			_, _ = fmt.Fprint(w, `<link rel="icon" sizes="48x48" href="/b.webp">`)
		case "/b.webp":
			imageFile(w, "image/webp", []byte("WEBP"))
		default:
			http.NotFound(w, r)
		}
	})

	assets := []model.Asset{
		{Ticker: "NVDA", Name: "NVIDIA", AssetType: "stock", ISIN: ptr("US67066G1040"), Enabled: true},
		{Ticker: "NOISIN", Name: "No ISIN", AssetType: "stock", Enabled: true},
		{Ticker: "OFF", Name: "Disabled", AssetType: "stock", Enabled: false},
	}
	_, err := st.SyncUnderlyings(ctx, assets)
	require.NoError(t, err)
	runID, err := st.CreateRun(ctx, store.RunParams{RunKey: "20261120T000000Z", Mode: "refresh", Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "test"})
	require.NoError(t, err)
	require.NoError(t, st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{
		{ID: "US67066G1040", CMCRwaID: ptr(2), CMCLogoURL: ptr(srv.URL + "/token.png"), LastRunID: runID},
		{ID: "stock:ticker:NOISIN", CMCRwaID: ptr(3), Website: ptr(srv.URL + "/issuer-b"), LastRunID: runID},
		{ID: "stock:ticker:OFF", CMCRwaID: ptr(4), CMCLogoURL: ptr(srv.URL + "/token.png"), LastRunID: runID},
	}))
	require.NoError(t, st.UpsertIssuers(ctx, []store.IssuerRow{
		{ID: "iss-a", CMCLogoURL: ptr(srv.URL + "/missing"), Website: ptr("http://issuer-a.test/"), MetadataStatus: "cmc"},
		{ID: "iss-b", Website: ptr(srv.URL + "/issuer-b"), MetadataStatus: "cmc"},
		{ID: "iss-none", MetadataStatus: "unknown"},
		{ID: "iss-html", CMCLogoURL: ptr(srv.URL + "/html-logo"), MetadataStatus: "cmc"},
		{ID: "iss-existing", CMCLogoURL: ptr(srv.URL + "/token.png"), MetadataStatus: "cmc"},
	}))
	require.NoError(t, st.UpsertTokens(ctx, runID, []store.TokenRow{
		{ID: 100, UnderlyingID: "US67066G1040", Kind: "tokenized", Coverage: "x", CMCLogoURL: ptr(srv.URL + "/token.png")},
		{ID: 101, UnderlyingID: "US67066G1040", Kind: "tokenized", Coverage: "x", CMCLogoURL: ptr(srv.URL + "/oversize.png")},
		{ID: 102, UnderlyingID: "US67066G1040", Kind: "tokenized", Coverage: "x"},
	}))
	inserted, err := st.InsertLogoIfAbsent(ctx, store.Logo{Kind: "issuers", EntityID: "iss-existing", Ext: "png", ContentType: "image/png",
		Bytes: []byte("MANUAL"), SHA256: "m", Source: "manual"})
	require.NoError(t, err)
	require.True(t, inserted)

	f := NewFetcher(pool, discard(), srv.Client())
	f.faviconFmt = srv.URL + "/favicons?domain=%s"
	rep, err := f.FetchMissing(ctx)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"tokens/100", "issuers/iss-a", "issuers/iss-b", "underlyings/US67066G1040", "underlyings/stock_ticker_NOISIN"}, rep.Downloaded)
	assert.ElementsMatch(t, []string{"tokens/101", "issuers/iss-html"}, rep.Failed)
	assert.ElementsMatch(t, []string{"tokens/102", "issuers/iss-none"}, rep.MissingSource)

	got, err := st.GetLogo(ctx, "tokens", "100")
	require.NoError(t, err)
	assert.Equal(t, pngBytes, got.Bytes)
	assert.Equal(t, "png", got.Ext)
	assert.Equal(t, "image/png", got.ContentType)
	assert.Equal(t, srv.URL+"/token.png", got.Source)
	assert.Len(t, got.SHA256, 64)

	got, err = st.GetLogo(ctx, "issuers", "iss-a")
	require.NoError(t, err)
	assert.Equal(t, "ico", got.Ext, "CMC logo 404 -> favicon service")
	assert.Equal(t, srv.URL+"/favicons?domain=issuer-a.test", got.Source)

	got, err = st.GetLogo(ctx, "issuers", "iss-b")
	require.NoError(t, err)
	assert.Equal(t, "webp", got.Ext, "favicon service 404 -> <link rel=icon>")
	assert.Equal(t, srv.URL+"/b.webp", got.Source)

	got, err = st.GetLogo(ctx, "underlyings", "stock_ticker_NOISIN")
	require.NoError(t, err)
	assert.Equal(t, "webp", got.Ext, "underlying entity id is the storage id")

	has, err := st.HasLogo(ctx, "underlyings", "stock_ticker_OFF")
	require.NoError(t, err)
	assert.False(t, has, "disabled underlyings are skipped")
	has, err = st.HasLogo(ctx, "tokens", "101")
	require.NoError(t, err)
	assert.False(t, has, "oversize rejected")

	existing, err := st.GetLogo(ctx, "issuers", "iss-existing")
	require.NoError(t, err)
	assert.Equal(t, []byte("MANUAL"), existing.Bytes, "an existing logo is never overwritten")
	assert.Equal(t, "manual", existing.Source)

	// Second pass: nothing new to download, failures are retried.
	rep, err = f.FetchMissing(ctx)
	require.NoError(t, err)
	assert.Empty(t, rep.Downloaded)
	assert.ElementsMatch(t, []string{"tokens/101", "issuers/iss-html"}, rep.Failed)
}

func TestFetchMissingCancelled(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewFetcher(pool, discard(), nil).FetchMissing(ctx)
	require.Error(t, err)
}
