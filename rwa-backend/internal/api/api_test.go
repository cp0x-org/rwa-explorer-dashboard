package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	mu        sync.Mutex
	builds    map[int64]*store.Build
	currentID int64
	pingErr   error
	runs      []store.Run
	changes   map[int64]store.RunBuildChanges
	runDetail map[int64]*store.RunDetail
	history   func(entity, id string, metrics []string, step string, limit int) (*store.HistoryResult, error)
	logos     map[string]*store.Logo
	requests  map[int64]*store.RunRequest
	lastSucc  time.Time
	lastRun   *store.Run
	events    map[int64][]store.RunEvent
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }
func (f *fakeStore) CurrentBuildMeta(context.Context) (*store.BuildMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.builds[f.currentID]
	if b == nil {
		return nil, store.ErrNotFound
	}
	m := b.BuildMeta
	return &m, nil
}
func (f *fakeStore) CurrentBuild(context.Context) (*store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.builds[f.currentID]; b != nil {
		return b, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) BuildByID(_ context.Context, id int64) (*store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.builds[id]; b != nil {
		return b, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) LastRun(context.Context) (*store.Run, error) {
	if f.lastRun == nil {
		return nil, store.ErrNotFound
	}
	return f.lastRun, nil
}
func (f *fakeStore) LastSuccessfulRunStart(context.Context) (time.Time, error) {
	if f.lastSucc.IsZero() {
		return time.Time{}, store.ErrNotFound
	}
	return f.lastSucc, nil
}
func (f *fakeStore) ListRuns(_ context.Context, limit int, cur *store.RunCursor) ([]store.Run, error) {
	var out []store.Run
	for _, r := range f.runs { // newest first
		if cur != nil && !r.StartedAt.Before(cur.StartedAt) && (!r.StartedAt.Equal(cur.StartedAt) || r.ID >= cur.ID) {
			continue
		}
		out = append(out, r)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (f *fakeStore) GetRun(_ context.Context, id int64) (*store.RunDetail, error) {
	if d := f.runDetail[id]; d != nil {
		return d, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) BuildChangesForRuns(_ context.Context, ids []int64) (map[int64]store.RunBuildChanges, error) {
	out := map[int64]store.RunBuildChanges{}
	for _, id := range ids {
		if c, ok := f.changes[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}
func (f *fakeStore) QueryHistory(_ context.Context, entity, id string, metrics []string, _, _ *time.Time, step string, limit int) (*store.HistoryResult, error) {
	return f.history(entity, id, metrics, step, limit)
}
func (f *fakeStore) GetLogo(_ context.Context, kind, id string) (*store.Logo, error) {
	if l := f.logos[kind+"/"+id]; l != nil {
		return l, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) CreateRunRequest(_ context.Context, by, mode string, symbols []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := int64(len(f.requests) + 1)
	if f.requests == nil {
		f.requests = map[int64]*store.RunRequest{}
	}
	f.requests[id] = &store.RunRequest{ID: id, RequestedBy: by, Mode: mode, Symbols: symbols, RequestedAt: time.Now()}
	return id, nil
}
func (f *fakeStore) GetRunRequest(_ context.Context, id int64) (*store.RunRequest, error) {
	if r := f.requests[id]; r != nil {
		return r, nil
	}
	return nil, store.ErrNotFound
}

func goldenBuild(t testing.TB, id int64) *store.Build {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "rwa.json"))
	require.NoError(t, err)
	// Stamp build_id/run_id into the bytes the way publishing does.
	raw = bytes.Replace(raw, []byte(`"schema_version"`), []byte(fmt.Sprintf(`"build_id": %d, "run_id": 7, "schema_version"`, id)), 1)
	sum := sha256.Sum256(raw)
	return &store.Build{
		BuildMeta: store.BuildMeta{ID: id, RunID: 7, GeneratedAt: time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC),
			SchemaVersion: 1, ScoreVersion: "v1", ExportSHA256: hex.EncodeToString(sum[:]), IsCurrent: true},
		Export: raw,
	}
}

func newFake(t testing.TB) *fakeStore {
	b := goldenBuild(t, 3)
	return &fakeStore{builds: map[int64]*store.Build{3: b}, currentID: 3, logos: map[string]*store.Logo{}}
}

func testServer(t testing.TB, f *fakeStore, mutate ...func(*config.Config)) (*Server, http.Handler) {
	t.Helper()
	cfg := &config.Config{HTTPAddr: "127.0.0.1:0"}
	for _, m := range mutate {
		m(cfg)
	}
	s := NewWithStore(cfg, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, s.cache.refresh(context.Background()))
	return s, s.Handler()
}

func do(h http.Handler, method, target string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func jsonBody(t testing.TB, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	body := rec.Body.Bytes()
	if rec.Header().Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err)
		body, err = io.ReadAll(zr)
		require.NoError(t, err)
	}
	require.NoError(t, json.Unmarshal(body, v), string(body))
}

func errCode(t testing.TB, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var e struct {
		Error map[string]any `json:"error"`
	}
	jsonBody(t, rec, &e)
	require.NotNil(t, e.Error)
	return e.Error
}

func TestExport(t *testing.T) {
	f := newFake(t)
	_, h := testServer(t, f)
	b := f.builds[3]
	etag := `"` + b.ExportSHA256 + `"`

	rec := do(h, "GET", "/api/v1/export", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, b.Export, rec.Body.Bytes(), "stored bytes verbatim")
	assert.Equal(t, etag, rec.Header().Get("ETag"))
	assert.Equal(t, "public, max-age=60, stale-while-revalidate=600", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rec.Header().Get("Content-Encoding"))

	// Verbatim bytes also carry build_id and run_id.
	var doc map[string]any
	jsonBody(t, rec, &doc)
	assert.EqualValues(t, 3, doc["build_id"])
	assert.EqualValues(t, 7, doc["run_id"])

	rec = do(h, "GET", "/api/v1/export", map[string]string{"If-None-Match": etag}, "")
	assert.Equal(t, 304, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	rec = do(h, "GET", "/api/v1/export", map[string]string{"If-None-Match": `"other", ` + etag}, "")
	assert.Equal(t, 304, rec.Code)
	rec = do(h, "GET", "/api/v1/export", map[string]string{"If-None-Match": `"other"`}, "")
	assert.Equal(t, 200, rec.Code)

	rec = do(h, "GET", "/api/v1/export", map[string]string{"Accept-Encoding": "gzip"}, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Equal(t, etag, rec.Header().Get("ETag"))
	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, b.Export, plain)
	assert.Less(t, rec.Body.Len(), len(b.Export))

	rec = do(h, "GET", "/api/v1/export", map[string]string{"Accept-Encoding": "gzip;q=0"}, "")
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
}

func TestExportByBuild(t *testing.T) {
	f := newFake(t)
	f.builds[2] = goldenBuild(t, 2)
	f.builds[2].IsCurrent = false
	_, h := testServer(t, f)

	rec := do(h, "GET", "/api/v1/export?build=2", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	assert.Equal(t, `"`+f.builds[2].ExportSHA256+`"`, rec.Header().Get("ETag"))
	assert.Equal(t, f.builds[2].Export, rec.Body.Bytes())

	rec = do(h, "GET", "/api/v1/export?build=3", nil, "")
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	rec = do(h, "GET", "/api/v1/export?build=2", map[string]string{"If-None-Match": `"` + f.builds[2].ExportSHA256 + `"`}, "")
	assert.Equal(t, 304, rec.Code)

	rec = do(h, "GET", "/api/v1/export?build=99", nil, "")
	require.Equal(t, 404, rec.Code)
	assert.Equal(t, "not_found", errCode(t, rec)["code"])
	assert.Empty(t, rec.Header().Get("ETag"))
	rec = do(h, "GET", "/api/v1/export?build=abc", nil, "")
	assert.Equal(t, 400, rec.Code)
}

func TestCacheRefreshSwapsBuild(t *testing.T) {
	f := newFake(t)
	s, h := testServer(t, f)
	f.mu.Lock()
	f.builds[4] = goldenBuild(t, 4)
	f.currentID = 4
	f.mu.Unlock()
	require.NoError(t, s.cache.refresh(context.Background()))
	rec := do(h, "GET", "/api/v1/export", nil, "")
	assert.Equal(t, `"`+f.builds[4].ExportSHA256+`"`, rec.Header().Get("ETag"))
}

func TestStatus(t *testing.T) {
	f := newFake(t)
	fin := time.Date(2026, 10, 18, 20, 30, 0, 0, time.UTC)
	f.lastRun = &store.Run{ID: 7, RunKey: "k", Mode: "refresh", Trigger: "schedule", Status: "partial",
		StartedAt: fin.Add(-time.Hour), FinishedAt: &fin, Counters: store.RunCounters{Completed: 120, Failed: 2}, CollectorVersion: "v2.0.1"}
	s, h := testServer(t, f, func(c *config.Config) { c.CollectCron = "0 6 * * *" })
	s.now = func() time.Time { return time.Date(2026, 10, 19, 7, 0, 0, 0, time.UTC) }

	rec := do(h, "GET", "/api/v1/status", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var st map[string]any
	jsonBody(t, rec, &st)
	cb := st["current_build"].(map[string]any)
	assert.EqualValues(t, 3, cb["id"])
	assert.Equal(t, f.builds[3].ExportSHA256, cb["export_sha256"])
	assert.EqualValues(t, 1, cb["schema_version"])
	lr := st["last_run"].(map[string]any)
	assert.Equal(t, "partial", lr["status"])
	assert.EqualValues(t, 120, lr["completed_count"])
	assert.Equal(t, "2026-10-20T06:00:00Z", st["next_run_at"])
	assert.Equal(t, map[string]any{"cron": "0 6 * * *", "interval": nil}, st["schedule"])
	assert.Equal(t, "v2.0.1", st["collector_version"])
	assert.Equal(t, "v1", st["api_version"])

	// Interval: last successful start plus the interval.
	f.lastSucc = time.Date(2026, 10, 19, 5, 0, 0, 0, time.UTC)
	s2, h2 := testServer(t, f, func(c *config.Config) { c.CollectInterval = 6 * time.Hour })
	s2.now = s.now
	rec = do(h2, "GET", "/api/v1/status", nil, "")
	jsonBody(t, rec, &st)
	assert.Equal(t, "2026-10-19T11:00:00Z", st["next_run_at"])
	assert.Equal(t, map[string]any{"cron": nil, "interval": "6h0m0s"}, st["schedule"])

	// No run yet: null last_run and collector_version.
	f.lastRun = nil
	rec = do(h, "GET", "/api/v1/status", nil, "")
	jsonBody(t, rec, &st)
	assert.Nil(t, st["last_run"])
	assert.Nil(t, st["collector_version"])
}

func TestSummaryAndUnderlyings(t *testing.T) {
	_, h := testServer(t, newFake(t))
	var sum map[string]any
	jsonBody(t, do(h, "GET", "/api/v1/summary", nil, ""), &sum)
	assert.EqualValues(t, 3, sum["build_id"])
	assert.NotNil(t, sum["summary"])
	assert.NotNil(t, sum["settings"])

	rec := do(h, "GET", "/api/v1/underlyings", nil, "")
	var list struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, rec, &list)
	require.Len(t, list.Items, 125)
	assert.Equal(t, "W/", rec.Header().Get("ETag")[:2])
	assert.Equal(t, "public, max-age=60, stale-while-revalidate=600", rec.Header().Get("Cache-Control"))

	// 304 on the weak etag.
	rec2 := do(h, "GET", "/api/v1/underlyings", map[string]string{"If-None-Match": rec.Header().Get("ETag")}, "")
	assert.Equal(t, 304, rec2.Code)

	ticker := list.Items[0]["ticker"].(string)
	for _, key := range []string{ticker, strings.ToLower(ticker), list.Items[0]["id"].(string)} {
		var one struct {
			Underlying map[string]any   `json:"underlying"`
			Tokens     []map[string]any `json:"tokens"`
			Excluded   []map[string]any `json:"excluded_tokens"`
		}
		rec := do(h, "GET", "/api/v1/underlyings/"+key, nil, "")
		require.Equal(t, 200, rec.Code, key)
		jsonBody(t, rec, &one)
		assert.Equal(t, list.Items[0]["id"], one.Underlying["id"])
		assert.NotNil(t, one.Tokens)
		assert.NotNil(t, one.Excluded)
		assert.Len(t, one.Tokens, len(list.Items[0]["token_ids"].([]any)))
	}
	rec = do(h, "GET", "/api/v1/underlyings/NOPE", nil, "")
	assert.Equal(t, 404, rec.Code)
	assert.Equal(t, "not_found", errCode(t, rec)["code"])
}

func TestTokensAndIssuers(t *testing.T) {
	s, h := testServer(t, newFake(t))
	sn := s.cache.current()

	var all struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, do(h, "GET", "/api/v1/tokens", nil, ""), &all)
	require.Len(t, all.Items, len(sn.export.Tokens))

	var defi struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, do(h, "GET", "/api/v1/tokens?in_defi=true", nil, ""), &defi)
	require.NotEmpty(t, defi.Items)
	require.Less(t, len(defi.Items), len(all.Items))
	for _, it := range defi.Items {
		assert.Greater(t, it["counts"].(map[string]any)["eligible_opportunities_count"], float64(0))
	}

	// Combined filters: underlying by ticker (case-insensitive) and kind.
	tok := sn.export.Tokens[0]
	url := fmt.Sprintf("/api/v1/tokens?underlying=%s&kind=%s", strings.ToLower(tok.Ticker), tok.Kind)
	var filt struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, do(h, "GET", url, nil, ""), &filt)
	require.NotEmpty(t, filt.Items)
	for _, it := range filt.Items {
		assert.Equal(t, tok.UnderlyingID, it["underlying_id"])
		assert.Equal(t, tok.Kind, it["kind"])
	}
	jsonBody(t, do(h, "GET", "/api/v1/tokens?underlying=NOPE", nil, ""), &filt)
	assert.Empty(t, filt.Items)
	assert.NotNil(t, filt.Items)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/tokens?in_defi=maybe", nil, "").Code)

	// Token with opportunities; unconfirmed ones carry relation null.
	var withUnconf *map[string]any
	for _, tk := range sn.export.Tokens {
		if len(tk.UnconfirmedKeys) > 0 && len(tk.Relations) > 0 {
			var detail map[string]any
			rec := do(h, "GET", fmt.Sprintf("/api/v1/tokens/%d", tk.ID), nil, "")
			require.Equal(t, 200, rec.Code)
			jsonBody(t, rec, &detail)
			withUnconf = &detail
			break
		}
	}
	require.NotNil(t, withUnconf, "golden has a token with relations and unconfirmed candidates")
	d := *withUnconf
	assert.NotNil(t, d["token"])
	assert.NotNil(t, d["underlying"])
	nullRel, setRel := 0, 0
	for _, o := range d["opportunities"].([]any) {
		m := o.(map[string]any)
		assert.NotNil(t, m["opportunity"])
		if m["relation"] == nil {
			nullRel++
		} else {
			setRel++
		}
	}
	assert.Positive(t, nullRel)
	assert.Positive(t, setRel)
	assert.Equal(t, 404, do(h, "GET", "/api/v1/tokens/1", nil, "").Code)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/tokens/abc", nil, "").Code)

	var iss struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, do(h, "GET", "/api/v1/issuers", nil, ""), &iss)
	require.Len(t, iss.Items, len(sn.export.Issuers))
	id := iss.Items[0]["id"].(string)
	var one struct {
		Issuer map[string]any   `json:"issuer"`
		Tokens []map[string]any `json:"tokens"`
	}
	rec := do(h, "GET", "/api/v1/issuers/"+id, nil, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &one)
	assert.Equal(t, id, one.Issuer["id"])
	assert.Len(t, one.Tokens, len(iss.Items[0]["token_ids"].([]any)))
	assert.Equal(t, 404, do(h, "GET", "/api/v1/issuers/nope", nil, "").Code)
}

type oppPage struct {
	Items []struct {
		TokenID      int64          `json:"token_id"`
		UnderlyingID string         `json:"underlying_id"`
		Opportunity  map[string]any `json:"opportunity"`
		Relation     map[string]any `json:"relation"`
	} `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func TestOpportunitiesPagination(t *testing.T) {
	s, h := testServer(t, newFake(t))
	total := len(s.cache.current().rows)
	require.Greater(t, total, 250)

	// Walk every page; the concatenation equals the full sorted table with no gaps or repeats.
	var seen []string
	cursor := ""
	pages := 0
	for {
		url := "/api/v1/opportunities?limit=97"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		rec := do(h, "GET", url, nil, "")
		require.Equal(t, 200, rec.Code)
		var p oppPage
		jsonBody(t, rec, &p)
		for _, it := range p.Items {
			seen = append(seen, fmt.Sprintf("%s|%d", it.Opportunity["key"], it.TokenID))
		}
		pages++
		if p.NextCursor == nil {
			break
		}
		cursor = *p.NextCursor
		require.Less(t, pages, total, "pagination must terminate")
	}
	require.Len(t, seen, total)
	for i, row := range s.cache.current().rows {
		assert.Equal(t, fmt.Sprintf("%s|%d", row.opp.Key, row.tokenID), seen[i])
	}

	// Default limit is 100; exact multiple has no dangling cursor.
	var p oppPage
	jsonBody(t, do(h, "GET", "/api/v1/opportunities", nil, ""), &p)
	assert.Len(t, p.Items, 100)
	require.NotNil(t, p.NextCursor)

	// Filters: link=unconfirmed has null relation; direct has depth 1; indirect depth > 1.
	jsonBody(t, do(h, "GET", "/api/v1/opportunities?link=unconfirmed&limit=50", nil, ""), &p)
	require.NotEmpty(t, p.Items)
	for _, it := range p.Items {
		assert.Nil(t, it.Relation)
	}
	jsonBody(t, do(h, "GET", "/api/v1/opportunities?link=direct&limit=50", nil, ""), &p)
	require.NotEmpty(t, p.Items)
	for _, it := range p.Items {
		assert.EqualValues(t, 1, it.Relation["depth"])
	}
	jsonBody(t, do(h, "GET", "/api/v1/opportunities?link=indirect&limit=1000", nil, ""), &p)
	for _, it := range p.Items {
		assert.Greater(t, it.Relation["depth"], float64(1))
	}
	first := s.cache.current().rows[0]
	jsonBody(t, do(h, "GET", "/api/v1/opportunities?network="+first.opp.Network+"&eligibility="+first.opp.Eligibility+"&limit=1000", nil, ""), &p)
	require.NotEmpty(t, p.Items)
	for _, it := range p.Items {
		assert.Equal(t, first.opp.Network, it.Opportunity["network"])
		assert.Equal(t, first.opp.Eligibility, it.Opportunity["eligibility"])
	}

	for _, bad := range []string{"cursor=!!!", "cursor=" + "YWJj", "limit=0", "limit=1001", "limit=x", "link=sideways"} {
		rec := do(h, "GET", "/api/v1/opportunities?"+bad, nil, "")
		assert.Equal(t, 400, rec.Code, bad)
		assert.Equal(t, "bad_request", errCode(t, rec)["code"])
	}
}

func TestRuns(t *testing.T) {
	f := newFake(t)
	base := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	for i := 5; i >= 1; i-- {
		fin := base.AddDate(0, 0, i).Add(90 * time.Second)
		r := store.Run{ID: int64(i), RunKey: fmt.Sprintf("k%d", i), Mode: "refresh", Trigger: "schedule", Status: "succeeded",
			StartedAt: base.AddDate(0, 0, i), FinishedAt: &fin, Warnings: json.RawMessage(`["w1","w2"]`),
			Counters: store.RunCounters{Selected: 10, Completed: 9, Failed: 1, CMCRequests: 3}}
		f.runs = append(f.runs, r)
	}
	running := store.Run{ID: 6, RunKey: "k6", Mode: "add", Trigger: "manual", Status: "running", StartedAt: base.AddDate(0, 0, 6), Warnings: json.RawMessage(`[]`)}
	f.runs = append([]store.Run{running}, f.runs...)
	f.changes = map[int64]store.RunBuildChanges{
		5: {BuildID: 9, HasChanges: true, BuildChangesRow: store.BuildChangesRow{NewTokens: []int32{1, 2}, NewIssuers: []string{"x"},
			NewPositionsCount: 4, GonePositionsCount: 1, EligibleDelta: 3, Details: []byte(`{"a":1}`)}},
		4: {BuildID: 8}, // build without a changes row
	}
	err := "boom"
	f.runDetail = map[int64]*store.RunDetail{5: {Run: f.runs[1], Underlyings: []store.RunUnderlying{
		{RunID: 5, UnderlyingID: "stock:ticker:ZZZ", Status: "failed", Error: &err},
	}}}
	_, h := testServer(t, f)

	type page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	var p page
	jsonBody(t, do(h, "GET", "/api/v1/runs?limit=4", nil, ""), &p)
	require.Len(t, p.Items, 4)
	require.NotNil(t, p.NextCursor)
	assert.EqualValues(t, 6, p.Items[0]["id"])
	assert.Nil(t, p.Items[0]["duration_seconds"])
	assert.Nil(t, p.Items[0]["build_id"])
	assert.Nil(t, p.Items[0]["changes"])
	assert.EqualValues(t, 90, p.Items[1]["duration_seconds"])
	assert.EqualValues(t, 2, p.Items[1]["warnings_count"])
	assert.EqualValues(t, 9, p.Items[1]["build_id"])
	assert.Equal(t, map[string]any{"new_tokens_count": float64(2), "removed_tokens_count": float64(0), "new_positions_count": float64(4),
		"gone_positions_count": float64(1), "eligible_delta": float64(3)}, p.Items[1]["changes"])
	assert.EqualValues(t, 8, p.Items[2]["build_id"])
	assert.Nil(t, p.Items[2]["changes"])

	var p2 page
	jsonBody(t, do(h, "GET", "/api/v1/runs?limit=4&cursor="+*p.NextCursor, nil, ""), &p2)
	require.Len(t, p2.Items, 2)
	assert.EqualValues(t, 2, p2.Items[0]["id"])
	assert.Nil(t, p2.NextCursor)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/runs?cursor=zzz", nil, "").Code)

	var d map[string]any
	rec := do(h, "GET", "/api/v1/runs/5", nil, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &d)
	assert.Equal(t, []any{"w1", "w2"}, d["warnings"])
	us := d["underlyings"].([]any)
	require.Len(t, us, 1)
	assert.Equal(t, "ZZZ", us[0].(map[string]any)["ticker"])
	assert.Equal(t, "boom", us[0].(map[string]any)["error"])
	ch := d["changes"].(map[string]any)
	assert.Equal(t, []any{float64(1), float64(2)}, ch["new_tokens"])
	assert.Equal(t, []any{}, ch["removed_tokens"])
	assert.Equal(t, map[string]any{"a": float64(1)}, ch["details"])

	assert.Equal(t, 404, do(h, "GET", "/api/v1/runs/99", nil, "").Code)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/runs/x", nil, "").Code)
}

func TestHistory(t *testing.T) {
	f := newFake(t)
	obs := time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC)
	var gotMetrics []string
	var gotLimit int
	var gotID string
	f.history = func(entity, id string, metrics []string, _ string, limit int) (*store.HistoryResult, error) {
		gotMetrics, gotLimit, gotID = metrics, limit, id
		for _, m := range metrics {
			if m == "nope" {
				return nil, &store.UnknownMetricError{Entity: entity, Metric: m}
			}
		}
		if id == "empty" {
			return &store.HistoryResult{Points: []store.HistoryPoint{}}, nil
		}
		return &store.HistoryResult{Truncated: limit == 2, Points: []store.HistoryPoint{
			{RunID: 1, ObservedAt: obs, Values: map[string]any{"market_cap_usd": "43702741.31", "eligible_opportunities_count": int32(13), "apy_percent": nil}},
		}}, nil
	}
	s, h := testServer(t, f)
	sn := s.cache.current()
	tok := sn.export.Tokens[0]

	var res map[string]any
	rec := do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=market_cap_usd,eligible_opportunities_count&step=day", tok.ID), nil, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &res)
	assert.Equal(t, "tokens", res["entity"])
	assert.Equal(t, fmt.Sprint(tok.ID), res["id"])
	assert.Equal(t, "day", res["step"])
	assert.Equal(t, false, res["truncated"])
	pt := res["points"].([]any)[0].(map[string]any)
	assert.Equal(t, "2026-10-18T20:00:00Z", pt["observed_at"])
	assert.Equal(t, map[string]any{"market_cap_usd": "43702741.31", "eligible_opportunities_count": float64(13), "apy_percent": nil}, pt["values"])
	assert.Equal(t, []string{"market_cap_usd", "eligible_opportunities_count"}, gotMetrics)
	assert.Equal(t, 1000, gotLimit)

	// Truncated and limit clamping.
	jsonBody(t, do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=market_cap_usd&limit=2", tok.ID), nil, ""), &res)
	assert.Equal(t, true, res["truncated"])
	assert.Equal(t, 400, do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=market_cap_usd&limit=99999", tok.ID), nil, "").Code)
	do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=market_cap_usd&limit=5000", tok.ID), nil, "")
	assert.Equal(t, 5000, gotLimit)

	// Ticker resolution for underlyings (case-insensitive), id echoed as the resolved underlying id.
	u := sn.export.Underlyings[0]
	jsonBody(t, do(h, "GET", "/api/v1/history/underlyings/"+strings.ToLower(u.Ticker)+"?metrics=market_cap_usd", nil, ""), &res)
	assert.Equal(t, u.ID, gotID)
	assert.Equal(t, u.ID, res["id"])

	// Summary: id is null.
	jsonBody(t, do(h, "GET", "/api/v1/history/summary?metrics=tokens_count", nil, ""), &res)
	assert.Nil(t, res["id"])
	assert.Equal(t, "summary", res["entity"])

	// Unknown metric: 400 with allowed list.
	rec = do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=nope", tok.ID), nil, "")
	require.Equal(t, 400, rec.Code)
	e := errCode(t, rec)
	assert.Equal(t, "unknown_metric", e["code"])
	assert.Equal(t, "nope", e["metric"])
	assert.Contains(t, e["allowed"], "market_cap_usd")

	// metrics required, bad step/from.
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/summary", nil, "").Code)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/summary?metrics=a&from=yesterday", nil, "").Code)

	// Unknown entity: no points and not in the current build -> 404; known with no points -> 200.
	rec = do(h, "GET", "/api/v1/history/underlyings/empty?metrics=market_cap_usd", nil, "")
	assert.Equal(t, 404, rec.Code)
	assert.Equal(t, 404, do(h, "GET", "/api/v1/history/issuers/empty?metrics=x_count", nil, "").Code)
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) {
		return &store.HistoryResult{Points: []store.HistoryPoint{}}, nil
	}
	rec = do(h, "GET", fmt.Sprintf("/api/v1/history/tokens/%d?metrics=market_cap_usd", tok.ID), nil, "")
	assert.Equal(t, 200, rec.Code)
	// Store-level errors map to statuses.
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) {
		return nil, store.ErrBadID
	}
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/tokens/abc?metrics=market_cap_usd", nil, "").Code)
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) {
		return nil, store.ErrUnknownStep
	}
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/summary?metrics=a&step=hour", nil, "").Code)
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) {
		return nil, errors.New("db down")
	}
	rec = do(h, "GET", "/api/v1/history/summary?metrics=a", nil, "")
	assert.Equal(t, 500, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db down")

	// Catalog.
	var cat struct {
		Entities map[string][]struct{ Name, Type string } `json:"entities"`
	}
	jsonBody(t, do(h, "GET", "/api/v1/history/metrics", nil, ""), &cat)
	require.Len(t, cat.Entities, 5)
	types := map[string]string{}
	for _, m := range cat.Entities["tokens"] {
		types[m.Name] = m.Type
	}
	assert.Equal(t, "money", types["market_cap_usd"])
	assert.Equal(t, "score", types["usefulness_score"])
	assert.Equal(t, "percent", types["max_reported_apy_percent"])
	assert.Equal(t, "count", types["eligible_opportunities_count"])
	assert.NotContains(t, types, "breakdowns")
}

func TestAdminRuns(t *testing.T) {
	f := newFake(t)
	// ADMIN_TOKEN unset: 404 whatever the headers.
	_, h := testServer(t, f)
	assert.Equal(t, 404, do(h, "POST", "/api/v1/admin/runs", map[string]string{"Authorization": "Bearer x"}, `{}`).Code)
	assert.Equal(t, 404, do(h, "GET", "/api/v1/admin/runs/1", nil, "").Code)

	_, h = testServer(t, f, func(c *config.Config) { c.AdminToken = "s3cret-token" })
	auth := map[string]string{"Authorization": "Bearer s3cret-token"}
	for _, hdr := range []map[string]string{nil, {"Authorization": "Bearer s3cret-toke"}, {"Authorization": "Bearer s3cret-token-x"},
		{"Authorization": "s3cret-token"}, {"Authorization": "Basic s3cret-token"}, {"Authorization": "Bearer "}} {
		rec := do(h, "POST", "/api/v1/admin/runs", hdr, `{"mode":"refresh"}`)
		assert.Equal(t, 401, rec.Code, "%v", hdr)
		assert.Equal(t, "unauthorized", errCode(t, rec)["code"])
	}
	assert.Empty(t, f.requests)

	rec := do(h, "POST", "/api/v1/admin/runs", auth, `{"mode":"add","symbols":["nvda"," AAPL "]}`)
	require.Equal(t, 202, rec.Code)
	var out map[string]any
	jsonBody(t, rec, &out)
	assert.EqualValues(t, 1, out["request_id"])
	assert.Equal(t, "add", f.requests[1].Mode)
	assert.Equal(t, []string{"NVDA", "AAPL"}, f.requests[1].Symbols)
	assert.Equal(t, "admin-api", f.requests[1].RequestedBy)

	// Defaults, empty body.
	rec = do(h, "POST", "/api/v1/admin/runs", auth, ``)
	require.Equal(t, 202, rec.Code)
	assert.Equal(t, "refresh", f.requests[2].Mode)

	for _, bad := range []string{`{"mode":"nuke"}`, `{"mode":"add"}`, `{"symbols":[""]}`, `not json`, `{"mode":"refresh","x":1}`} {
		assert.Equal(t, 400, do(h, "POST", "/api/v1/admin/runs", auth, bad).Code, bad)
	}

	rec = do(h, "GET", "/api/v1/admin/runs/1", auth, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &out)
	assert.EqualValues(t, 1, out["request_id"])
	assert.Nil(t, out["outcome"])
	assert.Nil(t, out["run_id"])
	assert.Equal(t, 401, do(h, "GET", "/api/v1/admin/runs/1", nil, "").Code)
	assert.Equal(t, 404, do(h, "GET", "/api/v1/admin/runs/77", auth, "").Code)
}

func TestHealthAndReady(t *testing.T) {
	f := newFake(t)
	_, h := testServer(t, f)
	assert.Equal(t, 200, do(h, "GET", "/healthz", nil, "").Code)
	assert.Equal(t, 200, do(h, "GET", "/readyz", nil, "").Code)

	f.pingErr = errors.New("down")
	rec := do(h, "GET", "/readyz", nil, "")
	assert.Equal(t, 503, rec.Code)
	assert.Equal(t, "unavailable", errCode(t, rec)["code"])
	assert.Equal(t, 200, do(h, "GET", "/healthz", nil, "").Code)

	// No build yet: readyz and current-build endpoints are 503, healthz stays 200.
	empty := &fakeStore{builds: map[int64]*store.Build{}}
	_, h = testServer(t, empty)
	rec = do(h, "GET", "/readyz", nil, "")
	assert.Equal(t, 503, rec.Code)
	assert.Equal(t, "unavailable", errCode(t, rec)["code"])
	assert.Equal(t, 503, do(h, "GET", "/api/v1/export", nil, "").Code)
	assert.Equal(t, 503, do(h, "GET", "/api/v1/tokens", nil, "").Code)
	assert.Equal(t, 200, do(h, "GET", "/healthz", nil, "").Code)
	rec = do(h, "GET", "/api/v1/status", nil, "")
	require.Equal(t, 200, rec.Code)
	var st map[string]any
	jsonBody(t, rec, &st)
	assert.Nil(t, st["current_build"])
}

func TestLogos(t *testing.T) {
	f := newFake(t)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	f.logos["tokens/36992"] = &store.Logo{Kind: "tokens", EntityID: "36992", Ext: "png", ContentType: "image/png", Bytes: png, SHA256: "abc123"}
	_, h := testServer(t, f)

	rec := do(h, "GET", "/logos/tokens/36992.png", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	assert.Equal(t, `"abc123"`, rec.Header().Get("ETag"))
	assert.Equal(t, png, rec.Body.Bytes())
	assert.Equal(t, 304, do(h, "GET", "/logos/tokens/36992.png", map[string]string{"If-None-Match": `"abc123"`}, "").Code)

	for _, p := range []string{"/logos/tokens/36992.jpg", "/logos/tokens/36992", "/logos/tokens/1.png", "/logos/bogus/36992.png", "/logos/tokens/.png"} {
		rec := do(h, "GET", p, nil, "")
		assert.Equal(t, 404, rec.Code, p)
		assert.Equal(t, "not_found", errCode(t, rec)["code"])
	}
}

func TestCORS(t *testing.T) {
	_, h := testServer(t, newFake(t), func(c *config.Config) { c.CORSOrigins = []string{"http://localhost:5173"} })
	rec := do(h, "GET", "/healthz", map[string]string{"Origin": "http://localhost:5173"}, "")
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Values("Vary"), "Origin")

	rec = do(h, "GET", "/healthz", map[string]string{"Origin": "http://localhost:5173.evil.com"}, "")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Values("Vary"), "Origin")

	rec = do(h, "OPTIONS", "/api/v1/export", map[string]string{"Origin": "http://localhost:5173", "Access-Control-Request-Method": "GET"}, "")
	assert.Equal(t, 204, rec.Code)
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")

	// Not configured: no CORS headers at all.
	_, h = testServer(t, newFake(t))
	rec = do(h, "GET", "/healthz", map[string]string{"Origin": "http://localhost:5173"}, "")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestRequestID(t *testing.T) {
	_, h := testServer(t, newFake(t))
	rec := do(h, "GET", "/healthz", map[string]string{"X-Request-ID": "abc-123"}, "")
	assert.Equal(t, "abc-123", rec.Header().Get("X-Request-ID"))
	rec = do(h, "GET", "/healthz", nil, "")
	assert.Len(t, rec.Header().Get("X-Request-ID"), 16)
	rec = do(h, "GET", "/healthz", map[string]string{"X-Request-ID": "bad id\x01"}, "")
	assert.NotEqual(t, "bad id\x01", rec.Header().Get("X-Request-ID"))
}

func TestRecoverReturnsJSON500(t *testing.T) {
	f := newFake(t)
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) { panic("kaboom") }
	_, h := testServer(t, f)
	rec := do(h, "GET", "/api/v1/history/summary?metrics=tokens_count", map[string]string{"X-Request-ID": "r1"}, "")
	require.Equal(t, 500, rec.Code)
	assert.Equal(t, "internal", errCode(t, rec)["code"])
	assert.NotContains(t, rec.Body.String(), "kaboom")
	assert.Equal(t, "r1", rec.Header().Get("X-Request-ID"))
}

func TestJSONGzipAndNotFound(t *testing.T) {
	_, h := testServer(t, newFake(t))
	rec := do(h, "GET", "/api/v1/underlyings", map[string]string{"Accept-Encoding": "gzip"}, "")
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	var list map[string]any
	jsonBody(t, rec, &list)
	assert.NotEmpty(t, list["items"])
	rec = do(h, "GET", "/healthz", map[string]string{"Accept-Encoding": "gzip"}, "")
	assert.Empty(t, rec.Header().Get("Content-Encoding"), "small bodies are not compressed")

	rec = do(h, "GET", "/api/v1/nope", nil, "")
	assert.Equal(t, 404, rec.Code)
	assert.Equal(t, "not_found", errCode(t, rec)["code"])
	rec = do(h, "GET", "/api/v1/openapi.yaml", nil, "")
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "openapi: 3.1.0")
}

func TestAccessLogOmitsQuery(t *testing.T) {
	var buf bytes.Buffer
	f := newFake(t)
	s := NewWithStore(&config.Config{}, f, slog.New(slog.NewJSONHandler(&buf, nil)))
	require.NoError(t, s.cache.refresh(context.Background()))
	do(s.Handler(), "GET", "/api/v1/history/summary?metrics=x&token=SECRET", map[string]string{"X-Request-ID": "rid", "Authorization": "Bearer SECRET"}, "")
	assert.Contains(t, buf.String(), `"request_id":"rid"`)
	assert.NotContains(t, buf.String(), "SECRET")
}
