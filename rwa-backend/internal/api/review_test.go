package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

func TestMethodNotAllowedAndUnknownPath(t *testing.T) {
	_, h := testServer(t, newFake(t), func(c *config.Config) { c.CORSOrigins = []string{"http://localhost:5173"} })
	rec := do(h, "POST", "/api/v1/export", nil, "")
	require.Equal(t, 405, rec.Code)
	assert.Contains(t, rec.Header().Get("Allow"), "GET")
	assert.Equal(t, "method_not_allowed", errCode(t, rec)["code"])
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	rec = do(h, "GET", "/api/v1/nope", nil, "")
	require.Equal(t, 404, rec.Code)
	assert.Equal(t, "not_found", errCode(t, rec)["code"])
	// Preflight for an allowed origin still works.
	rec = do(h, "OPTIONS", "/api/v1/export", map[string]string{"Origin": "http://localhost:5173", "Access-Control-Request-Method": "GET"}, "")
	assert.Equal(t, 204, rec.Code)
	assert.Equal(t, "no-referrer", do(h, "GET", "/healthz", nil, "").Header().Get("Referrer-Policy"))
}

func TestHeadAndGzipLength(t *testing.T) {
	_, h := testServer(t, newFake(t))
	rec := do(h, "HEAD", "/api/v1/underlyings", nil, "")
	assert.Equal(t, 200, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	rec = do(h, "HEAD", "/api/v1/export", nil, "")
	assert.Equal(t, 200, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	rec = do(h, "GET", "/api/v1/underlyings", map[string]string{"Accept-Encoding": "gzip"}, "")
	assert.Equal(t, fmt.Sprint(rec.Body.Len()), rec.Header().Get("Content-Length"))

	etag := do(h, "GET", "/api/v1/tokens", nil, "").Header().Get("ETag")
	rec = do(h, "GET", "/api/v1/tokens", map[string]string{"If-None-Match": etag}, "")
	assert.Equal(t, 304, rec.Code)
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")
	rec = do(h, "GET", "/api/v1/tokens/abc", map[string]string{"If-None-Match": etag}, "")
	assert.Equal(t, 400, rec.Code)
}

func TestHistoryRanges(t *testing.T) {
	f := newFake(t)
	f.history = func(string, string, []string, string, int) (*store.HistoryResult, error) {
		return &store.HistoryResult{Points: []store.HistoryPoint{}}, nil
	}
	_, h := testServer(t, f)
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/summary?metrics=a&from=2026-02-01&to=2026-01-01", nil, "").Code)
	assert.Equal(t, 200, do(h, "GET", "/api/v1/history/summary?metrics=a&from=2026-01-01&to=2026-01-01", nil, "").Code, "date-only to is inclusive")
	assert.Equal(t, 400, do(h, "GET", "/api/v1/history/summary?metrics=a&limit=5001", nil, "").Code)
	assert.Equal(t, 404, do(h, "GET", "/api/v1/history/opportunities/nope?metrics=a", nil, "").Code)
}

func TestAdminBearerCaseAndBody(t *testing.T) {
	f := newFake(t)
	_, h := testServer(t, f, func(c *config.Config) { c.AdminToken = "tok" })
	assert.Equal(t, 202, do(h, "POST", "/api/v1/admin/runs", map[string]string{"Authorization": "bearer tok"}, `{}`).Code)
	auth := map[string]string{"Authorization": "Bearer tok"}
	assert.Equal(t, 400, do(h, "POST", "/api/v1/admin/runs", auth, `{} {}`).Code)
	assert.Equal(t, 413, do(h, "POST", "/api/v1/admin/runs", auth, `{"symbols":["`+strings.Repeat("A", 20000)+`"]}`).Code)
}

func TestArchiveCoalescingAndLRU(t *testing.T) {
	f := newFake(t)
	for id := int64(10); id < 30; id++ {
		b := goldenBuild(t, id)
		b.IsCurrent = false
		f.builds[id] = b
	}
	s, h := testServer(t, f)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.Equal(t, 200, do(h, "GET", "/api/v1/export?build=10", nil, "").Code)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, s.cache.loads.Load(), "concurrent misses load once")

	// Fill to capacity, touch the oldest, add one more: the touched one survives, the next-oldest goes.
	for id := int64(11); id < 10+maxArchived; id++ {
		_, err := s.cache.byID(context.Background(), id)
		require.NoError(t, err)
	}
	_, err := s.cache.byID(context.Background(), 10)
	require.NoError(t, err)
	_, err = s.cache.byID(context.Background(), 25)
	require.NoError(t, err)
	s.cache.mu.Lock()
	_, has10 := s.cache.archived[10]
	_, has11 := s.cache.archived[11]
	n := len(s.cache.archived)
	s.cache.mu.Unlock()
	assert.True(t, has10)
	assert.False(t, has11)
	assert.Equal(t, maxArchived, n)

	_, err = s.cache.byID(context.Background(), 999)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestConcurrentRefreshAndRequests(t *testing.T) {
	f := newFake(t)
	s, h := testServer(t, f)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, p := range []string{"/api/v1/summary", "/api/v1/opportunities?limit=5", "/api/v1/export?build=3", "/readyz"} {
					assert.Equal(t, 200, do(h, "GET", p, nil, "").Code, p)
				}
			}
		}()
	}
	for id := int64(4); id < 8; id++ {
		f.mu.Lock()
		f.builds[id] = goldenBuild(t, id)
		f.currentID = id
		f.mu.Unlock()
		require.NoError(t, s.cache.refresh(context.Background()))
	}
	close(stop)
	wg.Wait()
}
