// Package httpx is the HTTP client shared by the CMC and Portals sources: per-provider rate limit,
// retries with backoff and Retry-After, request/credit counters and optional raw response capture.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// ErrAuth is matched by errors.Is for an APIError with HTTP status 401 or 403.
var ErrAuth = errors.New("authentication failed")

const (
	maxBodyBytes  = 64 << 20
	maxRetryAfter = 5 * time.Minute
)

// APIError is a failed provider call: HTTP status >= 400 (after retries), a network error (Status 0)
// or an error reported inside an HTTP 200 body.
type APIError struct {
	Provider string
	Status   int
	Message  string
	Err      error // underlying transport error, if any
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("%s: %s", e.Provider, e.Message)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Message)
}

// Unwrap returns the transport error, if any.
func (e *APIError) Unwrap() error { return e.Err }

// Is reports ErrAuth for 401 and 403.
func (e *APIError) Is(target error) bool {
	return target == ErrAuth && (e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden)
}

// Config configures one provider client.
type Config struct {
	Provider          string
	BaseURL           string
	Headers           map[string]string // fixed headers (auth); never logged or captured
	Timeout           time.Duration
	RequestsPerMinute int // minimum interval between request starts is 1m / RequestsPerMinute; <=0 disables
	MaxRetries        int
	// RawDir, when set, receives <RawDir>/<provider>/NNNN_<path>.json for every answered request.
	RawDir string
	// Logger receives the client's warnings (long Retry-After, raw capture failures); the collector passes the run's
	// logger so they reach the run journal. Nil means slog.Default().
	Logger *slog.Logger
	// OnRequest is called after every answered request (not for retried attempts).
	OnRequest func(provider, path string, params url.Values, status int, d time.Duration)
	// HTTPClient overrides the transport; its Timeout is set from Config.Timeout when zero.
	HTTPClient *http.Client
}

// Client is safe for concurrent use; the rate limit is shared by all callers.
type Client struct {
	cfg      Config
	hc       *http.Client
	interval time.Duration

	mu   sync.Mutex
	next time.Time // earliest start of the next request

	requests atomic.Int64
	credits  atomic.Int64

	// test seams
	now    func() time.Time
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func() time.Duration
}

// New creates a client.
func New(cfg Config) *Client {
	// Copy the caller's client (never mutate it). Redirects are not followed: Go would forward custom
	// auth headers such as X-CMC_PRO_API_KEY to another host. 1.x does not follow redirects either.
	hc := &http.Client{}
	if cfg.HTTPClient != nil {
		cp := *cfg.HTTPClient
		hc = &cp
	}
	if hc.Timeout == 0 {
		hc.Timeout = cfg.Timeout
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := &Client{
		cfg:    cfg,
		hc:     hc,
		now:    time.Now,
		sleep:  sleepCtx,
		jitter: func() time.Duration { return time.Duration(rand.Float64() * float64(time.Second)) }, //nolint:gosec // retry jitter, not a secret
	}
	if cfg.RequestsPerMinute > 0 {
		c.interval = time.Minute / time.Duration(cfg.RequestsPerMinute)
	}
	return c
}

// Provider returns the provider name.
func (c *Client) Provider() string { return c.cfg.Provider }

// Requests returns the number of HTTP requests started, retries included.
func (c *Client) Requests() int64 { return c.requests.Load() }

// Credits returns the credits reported by the provider so far (CMC status.credit_count).
func (c *Client) Credits() int64 { return c.credits.Load() }

// AddCredits adds provider-reported credits to the counter.
func (c *Client) AddCredits(n int64) { c.credits.Add(n) }

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// waitTurn reserves the next request slot and sleeps until it starts.
func (c *Client) waitTurn(ctx context.Context) error {
	if c.interval <= 0 {
		return ctx.Err()
	}
	c.mu.Lock()
	now := c.now()
	start := c.next
	if start.Before(now) {
		start = now
	}
	c.next = start.Add(c.interval)
	c.mu.Unlock()
	return c.sleep(ctx, start.Sub(now))
}

func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func (c *Client) logger() *slog.Logger {
	if c.cfg.Logger != nil {
		return c.cfg.Logger
	}
	return slog.Default()
}

func (c *Client) backoff(ctx context.Context, attempt int, retryAfter string) error {
	d := min(60*time.Second, time.Duration(1<<min(attempt, 6))*time.Second) + c.jitter()
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
		// A server's Retry-After is honoured (ignoring it invites bans), capped at maxRetryAfter.
		ra := min(time.Duration(secs)*time.Second, maxRetryAfter)
		if ra > 60*time.Second {
			c.logger().Warn("long Retry-After", "step", "collect."+c.cfg.Provider, "provider", c.cfg.Provider, "seconds", int(ra/time.Second))
		}
		d = max(d, ra)
	}
	return c.sleep(ctx, d)
}

// Get performs GET base+path?params with rate limiting and retries. It returns the final status and
// body; for status >= 400 the error is an *APIError and status/body are still returned. A cancelled
// context returns ctx.Err().
func (c *Client) Get(ctx context.Context, path string, params url.Values) (int, []byte, error) {
	target := strings.TrimRight(c.cfg.BaseURL, "/") + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	for attempt := 0; ; attempt++ {
		if err := c.waitTurn(ctx); err != nil {
			return 0, nil, err
		}
		seq := c.requests.Add(1)
		started := c.now()
		status, body, header, err := c.do(ctx, target)
		if err != nil {
			if ctx.Err() != nil {
				return 0, nil, ctx.Err()
			}
			if attempt >= c.cfg.MaxRetries {
				return 0, nil, &APIError{Provider: c.cfg.Provider, Message: "network error: " + err.Error(), Err: err}
			}
			if err := c.backoff(ctx, attempt, ""); err != nil {
				return 0, nil, err
			}
			continue
		}
		if retryable(status) && attempt < c.cfg.MaxRetries {
			if err := c.backoff(ctx, attempt, header.Get("Retry-After")); err != nil {
				return 0, nil, err
			}
			continue
		}
		c.saveRaw(seq, path, params, status, body)
		if c.cfg.OnRequest != nil {
			c.cfg.OnRequest(c.cfg.Provider, path, params, status, c.now().Sub(started))
		}
		if status >= 300 {
			msg := ErrorMessage(body)
			if status < 400 {
				msg = "unexpected redirect (not followed)"
			}
			return status, body, &APIError{Provider: c.cfg.Provider, Status: status, Message: msg}
		}
		if !json.Valid(body) {
			// Every provider endpoint answers JSON: a 2xx page of HTML (a proxy, a maintenance page) is a failure, not data.
			return status, body, &APIError{Provider: c.cfg.Provider, Status: status, Message: "non-JSON body"}
		}
		return status, body, nil
	}
}

// GetJSON is Get followed by JSON decoding into v.
func (c *Client) GetJSON(ctx context.Context, path string, params url.Values, v any) error {
	_, body, err := c.Get(ctx, path, params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return &APIError{Provider: c.cfg.Provider, Status: 200, Message: "invalid JSON body: " + truncate(string(body), 200), Err: err}
	}
	return nil
}

func (c *Client) do(ctx context.Context, target string) (int, []byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("build request: %w", redactURL(err))
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, nil, redactURL(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, body, resp.Header, nil
}

// redactURL drops the URL from *url.Error so query strings do not leak into logs.
func redactURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]+`)

type rawRecord struct {
	URL       string            `json:"url"`
	Params    map[string]string `json:"params"`
	Status    int               `json:"status"`
	FetchedAt string            `json:"fetched_at"`
	Body      json.RawMessage   `json:"body"`
}

// saveRaw writes the response without headers (auth lives only in headers). Failures are logged, not fatal.
func (c *Client) saveRaw(seq int64, path string, params url.Values, status int, body []byte) {
	if c.cfg.RawDir == "" {
		return
	}
	rec := rawRecord{
		URL:       strings.TrimRight(c.cfg.BaseURL, "/") + path,
		Params:    map[string]string{},
		Status:    status,
		FetchedAt: c.now().UTC().Format("2006-01-02T15:04:05Z"),
		Body:      body,
	}
	for k, v := range params {
		rec.Params[k] = strings.Join(v, ",")
	}
	if !json.Valid(body) {
		rec.Body, _ = json.Marshal(map[string]string{"_non_json_body": truncate(string(body), 2000)})
	}
	b, err := json.MarshalIndent(rec, "", " ")
	if err == nil {
		dir := filepath.Join(c.cfg.RawDir, c.cfg.Provider)
		name := fmt.Sprintf("%04d_%s.json", seq, strings.Trim(nonAlnum.ReplaceAllString(path, "_"), "_"))
		if err = os.MkdirAll(dir, 0o750); err == nil {
			err = os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o600)
		}
	}
	if err != nil {
		c.logger().Warn("raw capture failed", "step", "collect."+c.cfg.Provider, "provider", c.cfg.Provider, "err", err)
	}
}

// ErrorMessage extracts a human-readable message from an error body: status.error_message, then
// message, error; otherwise the body truncated to 500 characters.
func ErrorMessage(body []byte) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) == nil && obj != nil {
		if raw, ok := obj["status"]; ok {
			var st struct {
				ErrorMessage any `json:"error_message"`
			}
			if json.Unmarshal(raw, &st) == nil && truthy(st.ErrorMessage) {
				return fmt.Sprint(st.ErrorMessage)
			}
		}
		for _, k := range []string{"message", "error"} {
			if raw, ok := obj[k]; ok {
				var v any
				if json.Unmarshal(raw, &v) == nil && truthy(v) {
					if s, isStr := v.(string); isStr {
						return truncate(s, 500)
					}
					return truncate(string(raw), 500)
				}
			}
		}
	}
	return truncate(strings.TrimSpace(string(body)), 500)
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case float64:
		return x != 0
	}
	return true
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
