package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// ListRunEvents mirrors store.ListRunEvents on the in-memory events.
func (f *fakeStore) ListRunEvents(_ context.Context, runID, afterID int64, level string, limit int) ([]store.RunEvent, *int64, error) {
	rank := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	out := []store.RunEvent{}
	for _, e := range f.events[runID] {
		if e.ID > afterID && rank[e.Level] >= rank[level] {
			out = append(out, e)
		}
	}
	var next *int64
	if len(out) > limit {
		out = out[:limit]
		id := out[limit-1].ID
		next = &id
	}
	return out, next, nil
}

// RunStatus and RunJournalClosed mirror the store on the in-memory runs and events.
func (f *fakeStore) RunStatus(_ context.Context, id int64) (string, error) {
	d, ok := f.runDetail[id]
	if !ok {
		return "", store.ErrNotFound
	}
	return d.Status, nil
}

func (f *fakeStore) RunJournalClosed(_ context.Context, id int64) (bool, error) {
	for _, e := range f.events[id] {
		if e.Step == "run" && e.Message == "journal closed" {
			return true, nil
		}
	}
	return false, nil
}

type eventsPage struct {
	RunID     int64            `json:"run_id"`
	RunStatus string           `json:"run_status"`
	Open      bool             `json:"journal_open"`
	Items     []map[string]any `json:"items"`
	NextAfter *int64           `json:"next_after"`
}

func TestRunEvents(t *testing.T) {
	f := newFake(t)
	base := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	fin := base.Add(time.Minute)
	f.runDetail = map[int64]*store.RunDetail{
		7: {Run: store.Run{ID: 7, RunKey: "k7", Mode: "refresh", Trigger: "cli", Status: "succeeded", StartedAt: base, FinishedAt: &fin, Warnings: json.RawMessage(`[]`)},
			EventsCount: 5, ErrorsCount: 1},
		8: {Run: store.Run{ID: 8, RunKey: "k8", Mode: "refresh", Trigger: "cli", Status: "running", StartedAt: base, Warnings: json.RawMessage(`[]`)}},
	}
	f.events = map[int64][]store.RunEvent{}
	for i, l := range []string{"debug", "info", "info", "warn", "error"} {
		f.events[7] = append(f.events[7], store.RunEvent{ID: int64(10 + i), RunID: 7, TS: base.Add(time.Duration(i) * time.Second), Level: l,
			Step: "collect.cmc", Message: "m" + l, Attrs: json.RawMessage(`{"n":1}`)})
	}
	// run 9: finished, but its job still writes (no terminal event yet)
	f.runDetail[9] = &store.RunDetail{Run: store.Run{ID: 9, RunKey: "k9", Mode: "refresh", Trigger: "cli", Status: "succeeded", StartedAt: base, FinishedAt: &fin, Warnings: json.RawMessage(`[]`)}}
	f.events[9] = []store.RunEvent{{ID: 30, RunID: 9, TS: base, Level: "info", Step: "run", Message: "run finished", Attrs: json.RawMessage(`{}`)}}
	f.runDetail[10] = &store.RunDetail{Run: store.Run{ID: 10, RunKey: "k10", Mode: "refresh", Trigger: "cli", Status: "succeeded", StartedAt: base, FinishedAt: &fin, Warnings: json.RawMessage(`[]`)}}
	f.events[10] = []store.RunEvent{{ID: 40, RunID: 10, TS: base, Level: "info", Step: "run", Message: "journal closed", Attrs: json.RawMessage(`{}`)}}
	_, h := testServer(t, f)

	var p eventsPage
	rec := do(h, "GET", "/api/v1/runs/7/events", nil, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &p)
	assert.EqualValues(t, 7, p.RunID)
	assert.Equal(t, "succeeded", p.RunStatus)
	require.Len(t, p.Items, 4, "default level is info")
	assert.Nil(t, p.NextAfter)
	assert.Equal(t, "collect.cmc", p.Items[0]["step"])
	assert.Equal(t, map[string]any{"n": float64(1)}, p.Items[0]["attrs"])

	// level filter
	p = eventsPage{}
	jsonBody(t, do(h, "GET", "/api/v1/runs/7/events?level=warn", nil, ""), &p)
	require.Len(t, p.Items, 2)
	p = eventsPage{}
	jsonBody(t, do(h, "GET", "/api/v1/runs/7/events?level=debug", nil, ""), &p)
	require.Len(t, p.Items, 5)

	// cursor: strict id > after
	p = eventsPage{}
	jsonBody(t, do(h, "GET", "/api/v1/runs/7/events?level=debug&limit=2", nil, ""), &p)
	require.Len(t, p.Items, 2)
	require.NotNil(t, p.NextAfter)
	assert.EqualValues(t, 11, *p.NextAfter)
	var p2 eventsPage
	jsonBody(t, do(h, "GET", "/api/v1/runs/7/events?level=debug&limit=2&after=11", nil, ""), &p2)
	require.Len(t, p2.Items, 2)
	assert.EqualValues(t, 12, p2.Items[0]["id"])
	var p3 eventsPage
	jsonBody(t, do(h, "GET", "/api/v1/runs/7/events?level=debug&after=14", nil, ""), &p3)
	assert.NotNil(t, p3.Items, "items is [] not null")
	assert.Empty(t, p3.Items)

	// running: not cacheable
	rec = do(h, "GET", "/api/v1/runs/8/events", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	// finished but not closed: still open, not cacheable
	rec = do(h, "GET", "/api/v1/runs/9/events", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	p = eventsPage{}
	jsonBody(t, rec, &p)
	assert.True(t, p.Open)
	assert.Equal(t, "succeeded", p.RunStatus)

	// finished and closed: cacheable
	rec = do(h, "GET", "/api/v1/runs/10/events", nil, "")
	assert.Equal(t, "public, max-age=60", rec.Header().Get("Cache-Control"))
	p = eventsPage{}
	jsonBody(t, rec, &p)
	assert.False(t, p.Open)

	// errors
	assert.Equal(t, 404, do(h, "GET", "/api/v1/runs/99/events", nil, "").Code)
	for _, q := range []string{"limit=0", "limit=2001", "limit=x", "level=trace", "after=-1", "after=x"} {
		assert.Equal(t, 400, do(h, "GET", "/api/v1/runs/7/events?"+q, nil, "").Code, q)
	}
	assert.Equal(t, 400, do(h, "GET", "/api/v1/runs/x/events", nil, "").Code)
	assert.Equal(t, 200, do(h, "GET", "/api/v1/runs/7/events?limit=2000", nil, "").Code)

	// run detail carries the counters
	var d map[string]any
	jsonBody(t, do(h, "GET", "/api/v1/runs/7", nil, ""), &d)
	assert.EqualValues(t, 5, d["events_count"])
	assert.EqualValues(t, 1, d["errors_count"])
}
