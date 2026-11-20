// Package logos downloads logos of tokens, issuers and underlyings into the logos table (port of 1.x logos.py).
//
// Sources, first that works: the CMC logo URL; otherwise, for the official website listed in CMC, Google's favicon
// service (corporate sites often block scripts) and then the icon declared by the site itself. Existing rows are
// never overwritten, so a manually inserted logo overrides everything.
package logos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

const (
	// MaxBytes is the largest accepted logo.
	MaxBytes = 2_000_000
	// RequestTimeout bounds every HTTP request.
	RequestTimeout = 15 * time.Second

	userAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36"
	faviconService = "https://www.google.com/s2/favicons?sz=128&domain=%s"
	maxPageBytes   = 300_000
)

// extensions maps accepted content types to the logos.ext values.
var extensions = map[string]string{
	"image/png":                "png",
	"image/jpeg":               "jpg",
	"image/svg+xml":            "svg",
	"image/webp":               "webp",
	"image/x-icon":             "ico",
	"image/vnd.microsoft.icon": "ico",
	"image/gif":                "gif",
}

// allowedExt are the values the logos.ext column documents.
var allowedExt = map[string]bool{"png": true, "jpg": true, "svg": true, "webp": true, "ico": true, "gif": true}

// Report is the outcome of FetchMissing. Entries are "kind/entity_id".
type Report struct {
	Downloaded    []string
	Failed        []string
	MissingSource []string // no CMC logo and no website
}

// Fetcher downloads missing logos.
type Fetcher struct {
	store *store.Store
	log   *slog.Logger
	hc    *http.Client
	// faviconFmt is the favicon service URL with a %s for the domain (replaced in tests).
	faviconFmt string
}

// NewFetcher creates a Fetcher. httpClient may be nil (a default client following redirects is used).
func NewFetcher(pool *pgxpool.Pool, log *slog.Logger, httpClient *http.Client) *Fetcher {
	return newFetcher(store.New(pool), log, httpClient)
}

func newFetcher(st *store.Store, log *slog.Logger, httpClient *http.Client) *Fetcher {
	if log == nil {
		log = slog.Default()
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Fetcher{store: st, log: log, hc: httpClient, faviconFmt: faviconService}
}

// WithLogger returns a copy of the fetcher that logs to log (the collector passes one carrying the run id and
// step=logos, so the report lands in the run journal).
func (f *Fetcher) WithLogger(log *slog.Logger) *Fetcher {
	c := *f
	c.log = log
	return &c
}

// FetchMissing downloads a logo for every token, issuer and underlying without a logos row. Network errors only
// skip that logo; a database error or a cancelled context ends the call.
func (f *Fetcher) FetchMissing(ctx context.Context) (Report, error) {
	var rep Report
	jobs, err := f.store.LogoJobsMissing(ctx)
	if err != nil {
		return rep, fmt.Errorf("logos: %w", err)
	}
	f.log.Info("logos: to download", "count", len(jobs))
	for n, j := range jobs {
		if err := ctx.Err(); err != nil {
			return rep, fmt.Errorf("logos: %w", err)
		}
		name := j.Kind + "/" + j.EntityID
		logo, website := nonEmpty(j.LogoURL), nonEmpty(j.Website)
		if logo == "" && website == "" {
			rep.MissingSource = append(rep.MissingSource, name)
			f.log.Debug("logos: no logo or website in CMC, skipped", "progress", progress(n, len(jobs)), "entity", name)
			continue
		}
		img, source := f.fetch(ctx, logo, website)
		if img == nil {
			if err := ctx.Err(); err != nil {
				return rep, fmt.Errorf("logos: %w", err)
			}
			rep.Failed = append(rep.Failed, name)
			f.log.Info("logos: failed", "progress", progress(n, len(jobs)), "entity", name)
			continue
		}
		sum := sha256.Sum256(img.data)
		inserted, err := f.store.InsertLogoIfAbsent(ctx, store.Logo{Kind: j.Kind, EntityID: j.EntityID, Ext: img.ext,
			ContentType: img.contentType, Bytes: img.data, SHA256: hex.EncodeToString(sum[:]), Source: source})
		if err != nil {
			return rep, fmt.Errorf("logos: %w", err)
		}
		rep.Downloaded = append(rep.Downloaded, name)
		f.log.Debug("logos: saved", "progress", progress(n, len(jobs)), "entity", name, "ext", img.ext, "inserted", inserted)
	}
	doneLevel := slog.LevelInfo
	if len(rep.Failed) > 0 {
		doneLevel = slog.LevelWarn // the one summary event that surfaces failures at warn
	}
	f.log.Log(ctx, doneLevel, "logos: done", "downloaded", len(rep.Downloaded), "failed", len(rep.Failed),
		"missing_source", len(rep.MissingSource))
	f.log.Debug("logos: report", "downloaded", rep.Downloaded, "failed", rep.Failed, "missing_source", rep.MissingSource)
	return rep, nil
}

func progress(n, total int) string { return strconv.Itoa(n+1) + "/" + strconv.Itoa(total) }

func nonEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

type image struct {
	data        []byte
	ext         string
	contentType string
}

// fetch tries the CMC logo, the favicon service and the website's declared icon, in that order.
func (f *Fetcher) fetch(ctx context.Context, logo, website string) (*image, string) {
	try := func(url string, err error) (*image, string) {
		if err != nil || url == "" {
			return nil, ""
		}
		img, err := f.download(ctx, url)
		if err != nil {
			f.log.Debug("logos: candidate rejected", "url", url, "err", err)
			return nil, ""
		}
		return img, url
	}
	if logo != "" {
		if img, src := try(logo, nil); img != nil {
			return img, src
		}
	}
	if website != "" {
		if u, err := url.Parse(website); err == nil && u.Hostname() != "" {
			if img, src := try(fmt.Sprintf(f.faviconFmt, u.Hostname()), nil); img != nil {
				return img, src
			}
		}
		iconURL, err := f.websiteIconURL(ctx, website)
		if img, src := try(iconURL, err); img != nil {
			return img, src
		}
	}
	return nil, ""
}

// get performs a GET with the 1.x headers and a per-request timeout; the response body is read up to limit+1.
func (f *Fetcher) get(ctx context.Context, rawURL string, limit int64) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}
	return resp, data, nil
}

// download fetches an image: HTTP 200, an image/* content type, 1..MaxBytes bytes.
func (f *Fetcher) download(ctx context.Context, rawURL string) (*image, error) {
	resp, data, err := f.get(ctx, rawURL, MaxBytes)
	if err != nil {
		return nil, err
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	switch {
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	case !strings.HasPrefix(contentType, "image/"):
		return nil, fmt.Errorf("content type %q is not an image", contentType)
	case len(data) == 0:
		return nil, errors.New("empty body")
	case len(data) > MaxBytes:
		return nil, fmt.Errorf("larger than %d bytes", MaxBytes)
	}
	ext := extensions[contentType]
	if ext == "" {
		ext = strings.TrimPrefix(strings.ToLower(path.Ext(resp.Request.URL.Path)), ".")
	}
	if !allowedExt[ext] {
		ext = "png"
	}
	return &image{data: data, ext: ext, contentType: contentType}, nil
}

var (
	linkTag = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	attr    = regexp.MustCompile(`([a-zA-Z-]+)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	sizeRe  = regexp.MustCompile(`^(\d+)x\d+`)
)

// websiteIconURL is the best icon declared in the page head (the biggest declared size wins), or the site's
// /favicon.ico when none is declared. A non-200 page yields "".
func (f *Fetcher) websiteIconURL(ctx context.Context, website string) (string, error) {
	resp, data, err := f.get(ctx, website, maxPageBytes)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	if len(data) > maxPageBytes {
		data = data[:maxPageBytes]
	}
	return IconFromHTML(string(data), resp.Request.URL), nil
}

// IconFromHTML picks the icon URL declared by <link rel=icon> tags in page, resolved against base. Without any
// declared icon it returns base's /favicon.ico.
func IconFromHTML(page string, base *url.URL) string {
	best, bestSize := "", -1
	for _, tag := range linkTag.FindAllString(page, -1) {
		attrs := map[string]string{}
		for _, m := range attr.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = strings.Trim(m[2], `"'`)
		}
		rel := strings.ToLower(attrs["rel"])
		if !strings.Contains(rel, "icon") || attrs["href"] == "" {
			continue
		}
		var size int
		sizes := attrs["sizes"]
		switch m := sizeRe.FindStringSubmatch(sizes); {
		case sizes == "any":
			size = 512
		case m != nil:
			size, _ = strconv.Atoi(m[1])
		case strings.Contains(rel, "apple-touch-icon"):
			size = 57 // undeclared sizes rank below declared ones
		default:
			size = 32
		}
		if size > bestSize {
			if ref, err := url.Parse(attrs["href"]); err == nil {
				best, bestSize = base.ResolveReference(ref).String(), size
			}
		}
	}
	if best != "" {
		return best
	}
	return base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
}
