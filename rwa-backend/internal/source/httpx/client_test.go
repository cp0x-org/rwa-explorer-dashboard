package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClient has no real sleeping: sleeps are recorded, jitter is zero.
func testClient(t *testing.T, h http.Handler, mut func(*Config)) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := Config{Provider: "test", BaseURL: srv.URL, Headers: map[string]string{"X-Key": "secret-value"}, Timeout: 5 * time.Second, MaxRetries: 3}
	if mut != nil {
		mut(&cfg)
	}
	c := New(cfg)
	var mu sync.Mutex
	sleeps := &[]time.Duration{}
	c.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		*sleeps = append(*sleeps, d)
		mu.Unlock()
		return ctx.Err()
	}
	c.jitter = func() time.Duration { return 0 }
	return c, sleeps
}

func TestGetOK(t *testing.T) {
	var gotKey, gotQuery string
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotQuery = r.Header.Get("X-Key"), r.URL.RawQuery
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), nil)
	status, body, err := c.Get(context.Background(), "/p", url.Values{"a": {"1,2"}})
	require.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.JSONEq(t, `{"ok":true}`, string(body))
	assert.Equal(t, "secret-value", gotKey)
	assert.Equal(t, "a=1%2C2", gotQuery)
	assert.EqualValues(t, 1, c.Requests())
}

func TestRetryAfterHonoured(t *testing.T) {
	var n atomic.Int32
	c, sleeps := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}), nil)
	_, _, err := c.Get(context.Background(), "/p", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 2, c.Requests())
	assert.Equal(t, []time.Duration{7 * time.Second}, *sleeps) // base backoff is 1s, Retry-After 7s is larger
}

func TestRetry500ThenOK(t *testing.T) {
	var n atomic.Int32
	c, sleeps := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":1}`))
	}), nil)
	status, _, err := c.Get(context.Background(), "/p", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.EqualValues(t, 3, c.Requests())
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *sleeps) // 2^attempt
}

func TestRetriesExhausted503(t *testing.T) {
	var n atomic.Int32
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"overloaded"}`))
	}), nil)
	status, _, err := c.Get(context.Background(), "/p", nil)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 503, status)
	assert.Equal(t, 503, ae.Status)
	assert.Equal(t, "test", ae.Provider)
	assert.Equal(t, "overloaded", ae.Message)
	assert.False(t, errors.Is(err, ErrAuth))
	assert.EqualValues(t, 4, n.Load()) // 1 + MaxRetries
}

func TestAuthErrors(t *testing.T) {
	for _, code := range []int{401, 403} {
		c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"status":{"error_code":1002,"error_message":"API key missing."}}`))
		}), nil)
		_, _, err := c.Get(context.Background(), "/p", nil)
		require.ErrorIs(t, err, ErrAuth)
		var ae *APIError
		require.ErrorAs(t, err, &ae)
		assert.Equal(t, "API key missing.", ae.Message)
		assert.EqualValues(t, 1, c.Requests(), "401/403 are not retried")
	}
}

func TestNonJSONBody(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<html>bad request</html>"))
	}), nil)
	_, _, err := c.Get(context.Background(), "/p", nil)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "<html>bad request</html>", ae.Message)
}

func TestErrorMessage(t *testing.T) {
	long := make([]byte, 800)
	for i := range long {
		long[i] = 'x'
	}
	assert.Equal(t, "boom", ErrorMessage([]byte(`{"status":{"error_message":"boom"},"message":"other"}`)))
	assert.Equal(t, "m", ErrorMessage([]byte(`{"status":{"error_message":""},"message":"m"}`)))
	assert.Equal(t, "e", ErrorMessage([]byte(`{"error":"e"}`)))
	assert.Len(t, ErrorMessage(long), 500)
	assert.Equal(t, `{"a":1}`, ErrorMessage([]byte(`{"a":1}`)))
}

func TestRateLimiterSpacing(t *testing.T) {
	now := time.Unix(1000, 0)
	c, sleeps := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}), func(cfg *Config) { cfg.RequestsPerMinute = 20 }) // 3s interval
	c.now = func() time.Time { return now }
	for range 3 {
		_, _, err := c.Get(context.Background(), "/p", nil)
		require.NoError(t, err)
	}
	// Fake clock does not advance: request 2 waits 3s, request 3 waits 6s.
	assert.Equal(t, []time.Duration{0, 3 * time.Second, 6 * time.Second}, *sleeps)
}

func TestRateLimiterRealClock(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}), func(cfg *Config) { cfg.RequestsPerMinute = 600 }) // 100ms interval
	c.sleep = sleepCtx
	start := time.Now()
	for range 3 {
		_, _, err := c.Get(context.Background(), "/p", nil)
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, time.Since(start), 190*time.Millisecond)
}

func TestContextCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := New(Config{Provider: "test", BaseURL: srv.URL, Timeout: time.Second, MaxRetries: 5})
	ctx, cancel := context.WithCancel(context.Background())
	var sleeping atomic.Bool
	c.sleep = func(ctx context.Context, d time.Duration) error {
		if d > 0 {
			sleeping.Store(true)
			cancel() // cancelled while waiting for the backoff
		}
		return sleepCtx(ctx, d)
	}
	_, _, err := c.Get(ctx, "/p", nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, sleeping.Load())
	assert.EqualValues(t, 1, c.Requests())
}

func TestTransportErrorRetriedThenFails(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // connection refused
	c := New(Config{Provider: "test", BaseURL: base, Timeout: time.Second, MaxRetries: 2})
	c.sleep = func(ctx context.Context, d time.Duration) error { return nil }
	_, _, err := c.Get(context.Background(), "/p", url.Values{"k": {"v"}})
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 0, ae.Status)
	assert.Contains(t, ae.Message, "network error")
	assert.NotContains(t, ae.Message, "k=v", "query must not leak into the error")
	assert.EqualValues(t, 3, c.Requests())
}

func TestRawCaptureAndHook(t *testing.T) {
	dir := t.TempDir()
	var hookPath string
	var hookStatus int
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[1,2]}`))
	}), func(cfg *Config) {
		cfg.RawDir = dir
		cfg.OnRequest = func(p, path string, _ url.Values, status int, d time.Duration) {
			hookPath, hookStatus = p+path, status
			assert.GreaterOrEqual(t, d, time.Duration(0))
		}
	})
	_, _, err := c.Get(context.Background(), "/v5/real-world-assets/quotes/latest", url.Values{"symbol": {"NVDA"}})
	require.NoError(t, err)
	assert.Equal(t, "test/v5/real-world-assets/quotes/latest", hookPath)
	assert.Equal(t, 200, hookStatus)

	b, err := os.ReadFile(filepath.Join(dir, "test", "0001_v5_real_world_assets_quotes_latest.json"))
	require.NoError(t, err)
	var rec map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &rec))
	assert.ElementsMatch(t, []string{"url", "params", "status", "fetched_at", "body"}, keys(rec))
	assert.NotContains(t, string(b), "secret-value")
	assert.JSONEq(t, `{"symbol":"NVDA"}`, string(rec["params"]))
	assert.JSONEq(t, `{"data":[1,2]}`, string(rec["body"]))
}

func TestRawCaptureNonJSON(t *testing.T) {
	dir := t.TempDir()
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain text"))
	}), func(cfg *Config) { cfg.RawDir = dir })
	_, _, err := c.Get(context.Background(), "/x", nil)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, "test", "0001_x.json"))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"_non_json_body": "plain text"`)
}

func TestCredits(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	c.AddCredits(2)
	c.AddCredits(1)
	assert.EqualValues(t, 3, c.Credits())
}

func TestGetJSONInvalid(t *testing.T) {
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}), nil)
	var v map[string]any
	err := c.GetJSON(context.Background(), "/x", nil, &v)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Contains(t, ae.Message, "invalid JSON")
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBackoffCaps(t *testing.T) {
	c, sleeps := testClient(t, http.NotFoundHandler(), nil)
	ctx := context.Background()
	for _, attempt := range []int{0, 5, 6, 10, 100} {
		require.NoError(t, c.backoff(ctx, attempt, ""))
	}
	assert.Equal(t, []time.Duration{time.Second, 32 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second}, *sleeps)

	*sleeps = nil
	require.NoError(t, c.backoff(ctx, 0, "100000")) // server asks for more than the cap
	require.NoError(t, c.backoff(ctx, 0, "90"))     // honoured above the 60s backoff cap
	require.NoError(t, c.backoff(ctx, 0, "abc"))    // not seconds: ignored
	assert.Equal(t, []time.Duration{5 * time.Minute, 90 * time.Second, time.Second}, *sleeps)
}

func TestLimiterConcurrentDistinctSlots(t *testing.T) {
	now := time.Unix(1000, 0)
	c, sleeps := testClient(t, http.NotFoundHandler(), func(cfg *Config) { cfg.RequestsPerMinute = 20 })
	c.now = func() time.Time { return now }
	const n = 10
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, c.waitTurn(context.Background()))
		}()
	}
	wg.Wait()
	got := map[time.Duration]bool{}
	for _, d := range *sleeps {
		got[d] = true
	}
	require.Len(t, got, n, "each caller gets its own slot")
	for i := range n {
		assert.True(t, got[time.Duration(i)*3*time.Second], i)
	}
}

func TestHookAndCaptureOnlyForFinalAnswer(t *testing.T) {
	dir := t.TempDir()
	var n atomic.Int32
	var hooks int
	c, _ := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}), func(cfg *Config) {
		cfg.RawDir = dir
		cfg.OnRequest = func(string, string, url.Values, int, time.Duration) { hooks++ }
	})
	_, _, err := c.Get(context.Background(), "/x", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, hooks, "retried attempts are not reported (as in 1.x)")
	entries, err := os.ReadDir(filepath.Join(dir, "test"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "0002_x.json", entries[0].Name())
}

func TestRedirectNotFollowed(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Key") != "" {
			leaked.Store(true)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/elsewhere", http.StatusFound)
	}))
	defer first.Close()

	hc := &http.Client{}
	c := New(Config{Provider: "test", BaseURL: first.URL, Headers: map[string]string{"X-Key": "secret-value"},
		Timeout: time.Second, MaxRetries: 3, HTTPClient: hc})
	c.sleep = func(ctx context.Context, d time.Duration) error { return nil }
	status, _, err := c.Get(context.Background(), "/p", nil)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 302, status)
	assert.Equal(t, 302, ae.Status)
	assert.False(t, leaked.Load(), "the key must never reach the redirect target")
	assert.EqualValues(t, 1, c.Requests(), "3xx is not retried")
	assert.Nil(t, hc.CheckRedirect, "the caller's client is not mutated")
}
