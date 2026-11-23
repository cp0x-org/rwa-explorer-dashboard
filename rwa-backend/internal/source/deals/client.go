// Package deals is the cp0x deals source (deals.py): the public deals list, reduced to the deals the links config
// names, in the cache shape {fetched_at, deals} that publish reads.
package deals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
)

// URL is the public deals endpoint.
const URL = "https://cp0x.com/api/v1/deals"

const (
	timeout      = 30 * time.Second
	maxBodyBytes = 16 << 20
	timeFormat   = "2006-01-02T15:04:05Z"
)

// ErrDeals is a failed deals fetch or an unusable response (deals.py::DealsError).
var ErrDeals = errors.New("deals")

// Deal is one cached deal.
type Deal struct {
	Title   any     `json:"title"`
	Link    string  `json:"link"`
	Bonus   any     `json:"bonus"`
	PageURL *string `json:"page_url"`
}

// Cache is the deals cache document.
type Cache struct {
	FetchedAt string          `json:"fetched_at"`
	Deals     map[string]Deal `json:"deals"`
}

// JSON encodes the cache document.
func (c *Cache) JSON() ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("deals: encode cache: %w", err)
	}
	return b, nil
}

// Client fetches the deals. The zero value is not usable; use New.
type Client struct {
	url string
	hc  *http.Client
	now func() time.Time
}

// New returns a client for the public endpoint. hc may be nil (a client with the 30 s timeout of deals.py).
func New(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{url: URL, hc: hc, now: time.Now}
}

// WithURL overrides the endpoint (tests).
func (c *Client) WithURL(u string) *Client { c.url = u; return c }

// WithClock overrides the clock (tests).
func (c *Client) WithClock(now func() time.Time) *Client { c.now = now; return c }

// Fetch is deals.py::fetch_deals: one GET, any transport, status or JSON failure is an ErrDeals.
func (c *Client) Fetch(ctx context.Context) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", ErrDeals, err)
	}
	req.Header.Set("Accept", "application/json")
	// httpx's default in Python does not follow redirects: a 3xx is "HTTP 301", whatever client the caller supplied.
	hc := *c.hc
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %s", ErrDeals, redact(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrDeals, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", ErrDeals, err)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeals, err)
	}
	return v, nil
}

func redact(err error) string {
	// *url.Error text repeats the URL; the endpoint is public, but keep the message short and uniform.
	s := err.Error()
	if i := strings.LastIndex(s, "\": "); i >= 0 {
		return s[i+3:]
	}
	return s
}

// Parse is deals.py::parse_deals: it keeps the named deals with a clean HTTPS link and reports the slugs of named
// entries whose link is unusable.
func Parse(body any) (deals map[string]Deal, skipped []string, err error) {
	root, ok := body.(map[string]any)
	list, isList := root["deals"].([]any)
	if !ok || !isList {
		return nil, nil, fmt.Errorf("%w: invalid deals response: expected a deals array", ErrDeals)
	}
	deals = map[string]Deal{}
	for _, it := range list {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		slug, ok := item["slug"].(string)
		if !ok || slug == "" {
			continue
		}
		if !curated.CleanHTTPSURL(item["link"]) {
			skipped = append(skipped, slug)
			continue
		}
		d := Deal{Title: item["title"], Link: item["link"].(string), Bonus: item["bonus"]}
		if curated.CleanDealPageURL(item["page_url"], slug) {
			p := item["page_url"].(string)
			d.PageURL = &p
		}
		deals[slug] = d
	}
	return deals, skipped, nil
}

// Refresh is deals.py::refresh_deals: it fetches the deals and returns the cache document, keeping only the slugs in
// keep (nil keeps all). warning names the named deals whose link was unusable; both are empty on a fetch error, which
// is returned as the error (the caller keeps its previous cache).
func (c *Client) Refresh(ctx context.Context, keep map[string]bool) (cache *Cache, warning string, err error) {
	body, err := c.Fetch(ctx)
	if err != nil {
		return nil, "", err
	}
	deals, skipped, err := Parse(body)
	if err != nil {
		return nil, "", err
	}
	if keep != nil {
		for slug := range deals {
			if !keep[slug] {
				delete(deals, slug)
			}
		}
		skipped = slices.DeleteFunc(skipped, func(s string) bool { return !keep[s] })
	}
	cache = &Cache{FetchedAt: c.now().UTC().Format(timeFormat), Deals: deals}
	if len(skipped) > 0 {
		warning = "deals: skipped unusable links for " + strings.Join(skipped, ", ")
	}
	return cache, warning, nil
}
