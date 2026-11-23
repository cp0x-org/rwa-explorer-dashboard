package deals

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mirrors the deals tests of rwa-data-pipeline/tests/test_links.py.

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

func TestParseKeepsOnlyWellFormedHTTPSDeals(t *testing.T) {
	body := decode(t, `{"deals":[
		{"slug":"good","title":"Good","link":"https://good.example.org/r?x=1","bonus":"b","page_url":"https://cp0x.com/deals/good"},
		{"slug":"foreign-page","link":"https://a.example.org","page_url":"https://evil.example.org/deals/foreign-page"},
		{"slug":"plain","link":"http://plain.example.org"},
		{"slug":"nolink"},
		{"slug":"script","link":"javascript:alert(1)"},
		{"slug":"","link":"https://empty.example.org"},
		{"link":"https://noslug.example.org"},
		"junk",
		{"slug":5,"link":"https://num.example.org"}]}`)
	deals, skipped, err := Parse(body)
	require.NoError(t, err)
	assert.Equal(t, []string{"plain", "nolink", "script"}, skipped, "named entries with an unusable link are reported")
	require.Len(t, deals, 2)
	page := "https://cp0x.com/deals/good"
	assert.Equal(t, Deal{Title: "Good", Link: "https://good.example.org/r?x=1", Bonus: "b", PageURL: &page}, deals["good"])
	assert.Nil(t, deals["foreign-page"].PageURL, "only the cp0x page of that very deal is kept")

	for _, bad := range []string{`[]`, `{}`, `{"deals":{}}`, `{"deals":"x"}`, `null`} {
		_, _, err := Parse(decode(t, bad))
		require.ErrorIs(t, err, ErrDeals, bad)
		assert.EqualError(t, err, "deals: invalid deals response: expected a deals array", bad)
	}
}

func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(nil).WithURL(srv.URL).WithClock(func() time.Time { return time.Date(2026, 11, 14, 1, 7, 38, 0, time.UTC) })
}

func TestRefreshBuildsTheCacheAndKeepsOnlyNamedSlugs(t *testing.T) {
	var gotPath, gotAccept string
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept = r.URL.Path, r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"deals":[
			{"slug":"backpack","title":"Backpack","link":"https://backpack.exchange/refer/cp0x","bonus":"150k","page_url":"https://cp0x.com/deals/backpack"},
			{"slug":"hyperliquid","link":"http://insecure.example.org"},
			{"slug":"other","link":"https://other.example.org"},
			{"slug":"other-broken","link":"ftp://x"}]}`))
	})
	cache, warning, err := c.Refresh(context.Background(), map[string]bool{"backpack": true, "hyperliquid": true})
	require.NoError(t, err)
	assert.Equal(t, "/", gotPath)
	assert.Equal(t, "application/json", gotAccept)
	assert.Equal(t, "2026-11-14T01:07:38Z", cache.FetchedAt)
	require.Len(t, cache.Deals, 1)
	assert.Equal(t, "https://backpack.exchange/refer/cp0x", cache.Deals["backpack"].Link)
	assert.Equal(t, "deals: skipped unusable links for hyperliquid", warning, "unusable links of unnamed deals are not reported")

	b, err := cache.JSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"fetched_at":"2026-11-14T01:07:38Z","deals":{"backpack":{"title":"Backpack","link":"https://backpack.exchange/refer/cp0x","bonus":"150k","page_url":"https://cp0x.com/deals/backpack"}}}`, string(b))

	// keep == nil keeps everything; an empty set keeps nothing (a links config naming no deals).
	cache, _, err = c.Refresh(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, cache.Deals, 2)
	cache, warning, err = c.Refresh(context.Background(), map[string]bool{})
	require.NoError(t, err)
	assert.Empty(t, cache.Deals)
	assert.NotNil(t, cache.Deals)
	assert.Empty(t, warning)
}

func TestFetchTurnsEveryFailureIntoADealsError(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"status":   func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) },
		"redirect": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/x", http.StatusFound) },
		"not json": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>")) },
		"empty":    func(w http.ResponseWriter, r *http.Request) {},
	} {
		c := serve(t, h)
		cache, warning, err := c.Refresh(context.Background(), nil)
		require.ErrorIs(t, err, ErrDeals, name)
		assert.Nil(t, cache, name)
		assert.Empty(t, warning, name)
	}
	c := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	_, _, err := c.Refresh(context.Background(), nil)
	assert.EqualError(t, err, "deals: HTTP 503")

	// An unreachable host is a deals error too, and a cancelled context is reported as such.
	dead := New(nil).WithURL("http://127.0.0.1:1")
	_, err = dead.Fetch(context.Background())
	require.ErrorIs(t, err, ErrDeals)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = dead.Fetch(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDefaultEndpoint(t *testing.T) {
	assert.Equal(t, "https://cp0x.com/api/v1/deals", URL)
	assert.Equal(t, URL, New(nil).url)
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var hits int
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/x" {
			_, _ = w.Write([]byte(`{"deals":[]}`))
			return
		}
		http.Redirect(w, r, "/x", http.StatusMovedPermanently)
	})
	_, err := c.Fetch(context.Background())
	assert.EqualError(t, err, "deals: HTTP 301")
	assert.Equal(t, 1, hits)
}
