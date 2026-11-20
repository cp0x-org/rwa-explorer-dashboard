package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

type memSink struct {
	mu   sync.Mutex
	rows []store.RunEvent
}

func (m *memSink) InsertRunEvents(_ context.Context, rows []store.RunEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, rows...)
	return nil
}

func (m *memSink) events() []store.RunEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]store.RunEvent(nil), m.rows...)
}

func TestJobJournalsStepsOfTheRun(t *testing.T) {
	h := runlog.New(slog.NewTextHandler(io.Discard, nil), runlog.Options{Interval: time.Hour})
	sink := &memSink{}
	h.SetSink(sink)
	defer h.Close()

	f := &fake{}
	deps := f.deps()
	var mu sync.Mutex
	var logosRun, retentionCutoffs = int64(0), 0
	deps.Logos = func(ctx context.Context, _ string) error {
		mu.Lock()
		logosRun = runlog.RunID(ctx) // the run id travels in the context
		mu.Unlock()
		return nil
	}
	deps.EventsRetention = func(_ context.Context, cutoff time.Time) (int64, error) {
		mu.Lock()
		retentionCutoffs++
		mu.Unlock()
		assert.WithinDuration(t, time.Now().AddDate(0, 0, -30), cutoff, time.Minute)
		return 3, nil
	}
	s, err := New(Config{Interval: time.Hour, OnStart: true, RetentionDays: 90, EventsRetentionDays: 30}, deps, slog.New(h))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	require.Eventually(t, func() bool { return len(sink.events()) >= 4 }, 3*time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	mu.Lock()
	assert.EqualValues(t, 7, logosRun)
	assert.Equal(t, 1, retentionCutoffs)
	mu.Unlock()
	got := map[string][]string{}
	for _, e := range sink.events() {
		assert.EqualValues(t, 7, e.RunID, "events before the run exists have no run id and are not stored")
		got[e.Step] = append(got[e.Step], e.Message)
	}
	assert.Contains(t, got["scheduler"], "collection run ended, publishing")
	assert.Contains(t, got["scheduler"], "job ended")
	assert.Contains(t, got["retention"], "snapshot retention done")
	assert.Contains(t, got["retention"], "run events retention done")
}
