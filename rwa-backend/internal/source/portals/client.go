// Package portals is the Portals source: supported networks, platform names, exact token lookup and
// paginated text search. Token records are kept as raw JSON: the snapshot stores them verbatim.
package portals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
)

// BaseURL is the Portals API host.
const BaseURL = "https://api.portals.fi"

const (
	// LookupBatch is the number of network:address keys per ids= request.
	LookupBatch = 25
	// MaxPages caps pagination of one query.
	MaxPages = 100
)

// NewHTTPClient builds the httpx client for Portals from settings. rawDir may be empty.
func NewHTTPClient(apiKey string, s *model.Settings, rawDir string, onRequest func(string, string, url.Values, int, time.Duration)) *httpx.Client {
	return httpx.New(httpx.Config{
		Provider:          "portals",
		BaseURL:           BaseURL,
		Headers:           map[string]string{"Authorization": "Bearer " + apiKey},
		Timeout:           time.Duration(s.RequestTimeoutSeconds) * time.Second,
		RequestsPerMinute: s.RequestsPerMinute.Portals,
		MaxRetries:        s.MaxRetries,
		RawDir:            rawDir,
		OnRequest:         onRequest,
	})
}

// Client talks to Portals through an httpx client.
type Client struct{ api *httpx.Client }

// New wraps an httpx client configured for Portals.
func New(api *httpx.Client) *Client { return &Client{api: api} }

// Record is the part of a token record needed for deduplication.
type Record struct {
	Key string `json:"key"`
}

// SearchMeta is the bookkeeping of one paginated query.
type SearchMeta struct {
	TotalItems    *int `json:"total_items"`
	Pages         int  `json:"pages"`
	Received      int  `json:"received"`
	Complete      bool `json:"complete"`
	CountMismatch bool `json:"count_mismatch,omitempty"`
}

// FetchNetworks lists the supported network slugs.
func (c *Client) FetchNetworks(ctx context.Context) ([]string, error) {
	_, body, err := c.api.Get(ctx, "/v1/networks", nil)
	if err != nil {
		return nil, fmt.Errorf("portals networks: %w", err)
	}
	// The network list drives every lookup: anything but a nonempty list of nonempty strings is a failure, not "no networks".
	var items []any
	out := []string{}
	if json.Unmarshal(body, &items) == nil {
		for _, it := range items {
			s, ok := it.(string)
			if !ok || strings.TrimSpace(s) == "" {
				out = nil
				break
			}
			out = append(out, s)
		}
	}
	if len(items) == 0 || out == nil {
		return nil, fmt.Errorf("portals networks: %w", &httpx.APIError{Provider: "portals",
			Message: "invalid networks response: expected a nonempty list of nonempty strings"})
	}
	return out, nil
}

// FetchPlatformNames maps platform slug to display name (the slug when unnamed).
func (c *Client) FetchPlatformNames(ctx context.Context) (map[string]string, error) {
	// Get already fails a non-JSON 2xx body (an HTML page), which like Python is an error. A valid JSON body that is
	// not an array gives {} (portals.py::fetch_platform_names).
	_, body, err := c.api.Get(ctx, "/v2/platforms", nil)
	if err != nil {
		return nil, fmt.Errorf("portals platforms: %w", err)
	}
	var items []json.RawMessage
	if json.Unmarshal(body, &items) != nil {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	for _, raw := range items {
		var p map[string]any
		if json.Unmarshal(raw, &p) != nil {
			continue // non-object items are skipped
		}
		slug, _ := p["platform"].(string)
		if slug == "" {
			continue
		}
		name, _ := p["name"].(string)
		if name == "" {
			name = slug
		}
		out[slug] = name
	}
	return out, nil
}

// paginate reads zero-based pages with a constant limit until more=false, deduplicating by key.
func (c *Client) paginate(ctx context.Context, params url.Values, limit int) (map[string]json.RawMessage, SearchMeta, error) {
	items := map[string]json.RawMessage{}
	var total *int
	pages, more := 0, true
	for page := 0; more && pages < MaxPages; page++ {
		q := url.Values{}
		for k, v := range params {
			q[k] = v
		}
		q.Set("limit", strconv.Itoa(limit))
		q.Set("page", strconv.Itoa(page))
		var body struct {
			Tokens     []json.RawMessage `json:"tokens"`
			TotalItems *int              `json:"totalItems"`
			More       bool              `json:"more"`
		}
		if err := c.api.GetJSON(ctx, "/v2/tokens", q, &body); err != nil {
			return nil, SearchMeta{}, err
		}
		pages++
		for _, raw := range body.Tokens {
			var r Record
			if json.Unmarshal(raw, &r) == nil && r.Key != "" {
				items[r.Key] = slices.Clone(raw)
			}
		}
		total, more = body.TotalItems, body.More
	}
	meta := SearchMeta{TotalItems: total, Pages: pages, Received: len(items), Complete: !more}
	if total != nil && *total != len(items) {
		meta.CountMismatch = true
	}
	return items, meta, nil
}

// Lookup fetches exact records for network:address keys (ids= batches of 25). Only what Portals
// indexes is returned.
func (c *Client) Lookup(ctx context.Context, keys []string, limit int) (map[string]json.RawMessage, error) {
	unique := slices.Clone(keys)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	found := map[string]json.RawMessage{}
	for start := 0; start < len(unique); start += LookupBatch {
		batch := unique[start:min(start+LookupBatch, len(unique))]
		items, meta, err := c.paginate(ctx, url.Values{"ids": {strings.Join(batch, ",")}}, limit)
		if err != nil {
			return nil, fmt.Errorf("portals lookup: %w", err)
		}
		// An exact lookup that misses records would report indexed contracts as not indexed: refuse it.
		reported := "None"
		if meta.TotalItems != nil {
			reported = strconv.Itoa(*meta.TotalItems)
		}
		switch {
		case meta.TotalItems != nil && meta.Received < *meta.TotalItems:
			return nil, fmt.Errorf("portals lookup: %w", &httpx.APIError{Provider: "portals",
				Message: fmt.Sprintf("incomplete lookup (count_mismatch): received %d, reported %s", meta.Received, reported)})
		case !meta.Complete:
			return nil, fmt.Errorf("portals lookup: %w", &httpx.APIError{Provider: "portals",
				Message: fmt.Sprintf("incomplete lookup (MAX_PAGES reached): received %d, reported %s", meta.Received, reported)})
		}
		for k, v := range items {
			found[k] = v
		}
	}
	return found, nil
}

// Search runs a paginated text search (page=0.. while more, at most MaxPages pages).
func (c *Client) Search(ctx context.Context, term string, limit int) (map[string]json.RawMessage, SearchMeta, error) {
	items, meta, err := c.paginate(ctx, url.Values{"search": {term}}, limit)
	if err != nil {
		return nil, SearchMeta{}, fmt.Errorf("portals search %q: %w", term, err)
	}
	return items, meta, nil
}

// MinimalTerms drops terms that contain a shorter kept term (Portals search matches substrings of
// symbol/name, so such a term adds nothing). Order: length, upper-case, as given.
func MinimalTerms(terms []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, t := range terms {
		if t = strings.TrimSpace(t); t != "" && !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		a, b := unique[i], unique[j]
		if la, lb := utf8.RuneCountInString(a), utf8.RuneCountInString(b); la != lb {
			return la < lb
		}
		if ua, ub := strings.ToUpper(a), strings.ToUpper(b); ua != ub {
			return ua < ub
		}
		return a < b
	})
	var out []string
	for _, t := range unique {
		ut := strings.ToUpper(t)
		if !slices.ContainsFunc(out, func(o string) bool { return strings.Contains(ut, strings.ToUpper(o)) }) {
			out = append(out, t)
		}
	}
	return out
}
