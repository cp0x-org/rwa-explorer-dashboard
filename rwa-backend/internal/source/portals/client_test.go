package portals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/raw/portals/" + name + ".json")
	require.NoError(t, err)
	var rec struct {
		Body json.RawMessage `json:"body"`
	}
	require.NoError(t, json.Unmarshal(b, &rec))
	return rec.Body
}

type queries struct {
	mu sync.Mutex
	q  []map[string]string
}

func (qs *queries) add(r *http.Request) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	m := map[string]string{"_path": r.URL.Path}
	for k, v := range r.URL.Query() {
		m[k] = v[0]
	}
	qs.q = append(qs.q, m)
}

func newClient(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) (*Client, *httpx.Client, *queries) {
	t.Helper()
	qs := &queries{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qs.add(r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	api := httpx.New(httpx.Config{Provider: "portals", BaseURL: srv.URL, Headers: map[string]string{"Authorization": "Bearer k"},
		Timeout: 5 * time.Second, RequestsPerMinute: 60000, MaxRetries: 1})
	return New(api), api, qs
}

func tokensPage(keys []string, total int, more bool, page int) string {
	toks := make([]string, len(keys))
	for i, k := range keys {
		toks[i] = fmt.Sprintf(`{"key":%q,"symbol":"S%d"}`, k, i)
	}
	return fmt.Sprintf(`{"pageItems":%d,"totalItems":%d,"more":%t,"page":%d,"tokens":[%s]}`, len(keys), total, more, page, strings.Join(toks, ","))
}

func TestNetworksAndPlatforms(t *testing.T) {
	c, api, qs := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/networks" {
			_, _ = w.Write(fixture(t, "networks"))
			return
		}
		_, _ = w.Write(fixture(t, "platforms"))
	})
	nets, err := c.FetchNetworks(context.Background())
	require.NoError(t, err)
	assert.Len(t, nets, 13)
	assert.Contains(t, nets, "ethereum")
	names, err := c.FetchPlatformNames(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Ankr", names["ankr"])
	assert.Len(t, names, 6)
	assert.EqualValues(t, 2, api.Requests())
	assert.Equal(t, "/v2/platforms", qs.q[1]["_path"])
}

func TestPlatformNameFallsBackToSlug(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"platform":"a"},{"platform":"b","name":"B"},{"name":"no slug"},"junk"]`))
	})
	// Non-object elements are skipped, like in 1.x.
	got, err := c.FetchPlatformNames(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "a", "b": "B"}, got)

	c, _, _ = newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"platform":"a"},{"platform":"b","name":"B"},{"name":"no slug"}]`))
	})
	names, err := c.FetchPlatformNames(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "a", "b": "B"}, names)
}

func TestLookupRecorded(t *testing.T) {
	c, _, qs := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(fixture(t, "tokens_lookup")) })
	keys := []string{"bsc:0xc845b2894dbddd03858fd2d643b4ef725fe0849d", "base:0xb20000000000000000000078ee7ce2fe4908108c"}
	got, err := c.Lookup(context.Background(), keys, 250)
	require.NoError(t, err)
	assert.Len(t, got, 10)
	assert.Contains(t, got, "bsc:0xc845b2894dbddd03858fd2d643b4ef725fe0849d")
	assert.Equal(t, "250", qs.q[0]["limit"])
	assert.Equal(t, "0", qs.q[0]["page"])
	// Records are verbatim: fields outside the light Record struct are preserved.
	assert.Contains(t, string(got["bsc:0xc845b2894dbddd03858fd2d643b4ef725fe0849d"]), `"tokenId"`)
}

func TestLookupBatchingAndDedupe(t *testing.T) {
	c, _, qs := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		_, _ = w.Write([]byte(tokensPage(ids, len(ids), false, 0)))
	})
	var keys []string
	for i := range 60 {
		keys = append(keys, "ethereum:0x"+strconv.Itoa(1000+i))
	}
	keys = append(keys, keys[0], keys[1]) // duplicates are dropped
	got, err := c.Lookup(context.Background(), keys, 250)
	require.NoError(t, err)
	assert.Len(t, got, 60)
	require.Len(t, qs.q, 3)
	sizes := []int{}
	for _, q := range qs.q {
		sizes = append(sizes, len(strings.Split(q["ids"], ",")))
	}
	assert.Equal(t, []int{25, 25, 10}, sizes)
}

func TestSearchPaginationMoreThenDone(t *testing.T) {
	c, api, qs := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		switch page {
		case 0:
			_, _ = w.Write([]byte(tokensPage([]string{"a:1", "a:2"}, 5, true, 0)))
		case 1:
			_, _ = w.Write([]byte(tokensPage([]string{"a:3", "a:2"}, 5, true, 1))) // a:2 repeated
		default:
			_, _ = w.Write([]byte(tokensPage([]string{"a:5"}, 5, false, 2)))
		}
	})
	got, meta, err := c.Search(context.Background(), "NVDA", 2)
	require.NoError(t, err)
	assert.Len(t, got, 4)
	require.NotNil(t, meta.TotalItems)
	assert.Equal(t, 5, *meta.TotalItems)
	assert.Equal(t, 3, meta.Pages)
	assert.Equal(t, 4, meta.Received)
	assert.True(t, meta.Complete)
	assert.True(t, meta.CountMismatch, "5 reported, 4 unique received")
	assert.EqualValues(t, 3, api.Requests())
	assert.Equal(t, "NVDA", qs.q[2]["search"])
	assert.Equal(t, "2", qs.q[2]["page"])
	assert.Equal(t, "2", qs.q[2]["limit"])
}

func TestSearchRecordedSinglePage(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(fixture(t, "tokens_search_aapl")) })
	got, meta, err := c.Search(context.Background(), "AAPL", 250)
	require.NoError(t, err)
	assert.Len(t, got, 6)
	assert.Equal(t, 1, meta.Pages)
	assert.True(t, meta.Complete)
	// The fixture was trimmed to 6 of 95 tokens, so the recorded totalItems no longer matches.
	assert.True(t, meta.CountMismatch)
}

func TestSearchMaxPages(t *testing.T) {
	c, api, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		_, _ = w.Write([]byte(tokensPage([]string{"k:" + strconv.Itoa(page)}, 100000, true, page)))
	})
	got, meta, err := c.Search(context.Background(), "X", 1)
	require.NoError(t, err)
	assert.Len(t, got, MaxPages)
	assert.Equal(t, MaxPages, meta.Pages)
	assert.False(t, meta.Complete)
	assert.EqualValues(t, MaxPages, api.Requests())
}

func TestSearchSkipsKeylessTokens(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalItems":2,"more":false,"tokens":[{"key":"a:1"},{"symbol":"nokey"},"junk"]}`))
	})
	got, meta, err := c.Search(context.Background(), "X", 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.True(t, meta.CountMismatch)
}

func TestAuthError(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	})
	_, _, err := c.Search(context.Background(), "X", 10)
	require.ErrorIs(t, err, httpx.ErrAuth)
	_, err = c.FetchNetworks(context.Background())
	require.ErrorIs(t, err, httpx.ErrAuth)
}

func TestMinimalTerms(t *testing.T) {
	assert.Equal(t, []string{"NVDA", "NVIDIA"}, MinimalTerms([]string{"NVDAon", "NVDA", "nvda.d", "NVIDIA", " NVDAx ", "", "NVDA"}))
	// Equal length sorts by upper-case, then as given; a kept term hides longer ones containing it.
	assert.Equal(t, []string{"AB", "cd"}, MinimalTerms([]string{"cd", "AB", "xABy", "ab"}))
	assert.Equal(t, []string{"Ab", "CD"}, MinimalTerms([]string{"CD", "Ab", "Ab"}))
	assert.Empty(t, MinimalTerms(nil))
}

// /v1/networks must be a nonempty list of nonempty strings: an empty or odd answer would silently turn every
// contract into "unsupported network", so it is an error (1.x returned an empty list).
func TestNetworksStrict(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"object", `{"error":"x"}`},
		{"empty list", `[]`},
		{"non-string item", `["ethereum", 5]`},
		{"empty string", `["ethereum", ""]`},
		{"blank string", `["ethereum", "  "]`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.body)) })
			nets, err := c.FetchNetworks(context.Background())
			var ae *httpx.APIError
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, "invalid networks response: expected a nonempty list of nonempty strings", ae.Message)
			assert.Nil(t, nets)
		})
	}
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`["ethereum","base"]`)) })
	nets, err := c.FetchNetworks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"ethereum", "base"}, nets)
}

// An exact lookup that did not return everything Portals reports (or hit the page cap) would turn indexed contracts
// into "not indexed": it fails instead.
func TestLookupIncomplete(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tokensPage([]string{"a:1"}, 3, false, 0)))
	})
	_, err := c.Lookup(context.Background(), []string{"a:1", "a:2", "a:3"}, 10)
	var ae *httpx.APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "incomplete lookup (count_mismatch): received 1, reported 3", ae.Message)

	c, _, _ = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		_, _ = w.Write([]byte(`{"more":true,"tokens":[{"key":"k:` + strconv.Itoa(page) + `"}]}`))
	})
	_, err = c.Lookup(context.Background(), []string{"a:1"}, 1)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "incomplete lookup (MAX_PAGES reached): received 100, reported None", ae.Message)

	// More records than reported (duplicates, a moving index) is not incomplete.
	c, _, _ = newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(tokensPage([]string{"a:1", "a:2"}, 1, false, 0)))
	})
	got, err := c.Lookup(context.Background(), []string{"a:1", "a:2"}, 10)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestNonArrayPlatformsGiveEmpty(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"error":"x"}`)) })
	names, err := c.FetchPlatformNames(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, names)
	assert.Empty(t, names)
}

func TestEmptyResultsAreNotNil(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"tokens":[],"more":false}`)) })
	found, err := c.Lookup(context.Background(), nil, 10)
	require.NoError(t, err)
	assert.NotNil(t, found)
	items, _, err := c.Search(context.Background(), "X", 10)
	require.NoError(t, err)
	assert.NotNil(t, items)
}

func TestHTMLPlatformsBodyIsAnError(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html>maintenance</html>`)) })
	_, err := c.FetchPlatformNames(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-JSON body")
}
