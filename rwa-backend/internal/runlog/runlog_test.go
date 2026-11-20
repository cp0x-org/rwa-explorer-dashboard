package runlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

type fakeSink struct {
	mu      sync.Mutex
	batches [][]store.RunEvent
	err     error
}

func (f *fakeSink) InsertRunEvents(_ context.Context, rows []store.RunEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.batches = append(f.batches, append([]store.RunEvent(nil), rows...))
	return nil
}

func (f *fakeSink) all() []store.RunEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.RunEvent
	for _, b := range f.batches {
		out = append(out, b...)
	}
	return out
}

func (f *fakeSink) batchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakeSink) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

// newTest returns a logger over a Handler with a fake sink; the stdout handler writes JSON at info into out.
func newTest(t *testing.T, opts Options) (*slog.Logger, *Handler, *fakeSink, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	h := New(slog.NewJSONHandler(&lockedWriter{w: out}, &slog.HandlerOptions{Level: slog.LevelInfo}), opts)
	sink := &fakeSink{}
	h.SetSink(sink)
	t.Cleanup(h.Close)
	return slog.New(h), h, sink, out
}

type lockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func attrsOf(t *testing.T, e store.RunEvent) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(e.Attrs, &m))
	return m
}

func TestForwardsAndJournalsOnlyRunRecords(t *testing.T) {
	log, h, sink, out := newTest(t, Options{Interval: time.Hour})
	log.Info("no run")
	log.Info("with attr", "run_id", 5, "step", "publish", "count", 3, "ok", true, "name", "x")
	log.With("run_id", int64(6), "step", "collect.cmc").Warn("derived", "provider", "cmc")
	log.InfoContext(WithRun(context.Background(), 7), "from context")
	require.NoError(t, h.Flush(context.Background()))

	ev := sink.all()
	require.Len(t, ev, 3)
	assert.EqualValues(t, 5, ev[0].RunID)
	assert.Equal(t, "publish", ev[0].Step)
	assert.Equal(t, "info", ev[0].Level)
	assert.Equal(t, map[string]any{"count": float64(3), "ok": true, "name": "x"}, attrsOf(t, ev[0]))
	assert.EqualValues(t, 6, ev[1].RunID)
	assert.Equal(t, "collect.cmc", ev[1].Step)
	assert.Equal(t, "warn", ev[1].Level)
	assert.Equal(t, map[string]any{"provider": "cmc"}, attrsOf(t, ev[1]))
	assert.EqualValues(t, 7, ev[2].RunID)
	assert.Equal(t, "run", ev[2].Step, "default step")
	assert.Equal(t, "{}", string(ev[2].Attrs))

	// stdout got every record once
	assert.Equal(t, 4, bytes.Count(out.Bytes(), []byte("\n")))
}

func TestLevelFilterAndStdoutLevelIndependent(t *testing.T) {
	// events at info: debug is neither stored nor printed
	log, h, sink, out := newTest(t, Options{Level: slog.LevelInfo, Interval: time.Hour})
	log.Debug("d", "run_id", 1)
	log.Info("i", "run_id", 1)
	log.Error("e", "run_id", 1)
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 2)
	assert.Equal(t, "info", ev[0].Level)
	assert.Equal(t, "error", ev[1].Level)

	// events at debug while stdout stays at info: debug is stored, not printed
	log, h, sink, out = newTest(t, Options{Level: slog.LevelDebug, Interval: time.Hour})
	log.Debug("d", "run_id", 1)
	log.Info("i", "run_id", 1)
	log.Debug("no run")
	require.NoError(t, h.Flush(context.Background()))
	ev = sink.all()
	require.Len(t, ev, 2)
	assert.Equal(t, "debug", ev[0].Level)
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("\n")), "only the info record is printed")

	// events at warn
	log, h, sink, _ = newTest(t, Options{Level: slog.LevelWarn, Interval: time.Hour})
	log.Info("i", "run_id", 1)
	log.Warn("w", "run_id", 1)
	require.NoError(t, h.Flush(context.Background()))
	assert.Len(t, sink.all(), 1)
}

func TestRedactionAndAttrConversion(t *testing.T) {
	log, h, sink, _ := newTest(t, Options{Interval: time.Hour})
	log.Info("m", "run_id", 1,
		"api_key", "secret1", "Authorization", "Bearer abc", "access_token", "t", "db_password", "p",
		"tokens", 12, // a counter, not a secret
		"took", 1500*time.Millisecond, "err", errors.New("boom"), "list", []string{"a", "b"},
		slog.Group("g", slog.String("k", "v"), slog.String("secret_key", "s")),
		"run_key", "2026")
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 1)
	got := attrsOf(t, ev[0])
	assert.Equal(t, "[redacted]", got["api_key"])
	assert.Equal(t, "[redacted]", got["Authorization"])
	assert.Equal(t, "[redacted]", got["access_token"])
	assert.Equal(t, "[redacted]", got["db_password"])
	assert.EqualValues(t, 12, got["tokens"])
	assert.EqualValues(t, 1500, got["took_ms"])
	assert.Equal(t, "boom", got["err"])
	assert.Equal(t, []any{"a", "b"}, got["list"])
	assert.Equal(t, "v", got["g.k"])
	assert.Equal(t, "[redacted]", got["g.secret_key"])
	assert.NotContains(t, got, "run_key")
}

func TestBatchFlushBySize(t *testing.T) {
	log, _, sink, _ := newTest(t, Options{BatchSize: 5, Interval: time.Hour})
	for i := 0; i < 5; i++ {
		log.Info("e", "run_id", 1, "i", i)
	}
	require.Eventually(t, func() bool { return len(sink.all()) == 5 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, sink.batchCount(), "one batch")
}

func TestFlushByInterval(t *testing.T) {
	log, _, sink, _ := newTest(t, Options{BatchSize: 1000, Interval: 20 * time.Millisecond})
	log.Info("e", "run_id", 1)
	require.Eventually(t, func() bool { return len(sink.all()) == 1 }, 2*time.Second, 5*time.Millisecond)
}

func TestCloseFlushes(t *testing.T) {
	log, h, sink, _ := newTest(t, Options{BatchSize: 1000, Interval: time.Hour})
	log.Info("e", "run_id", 1)
	assert.Empty(t, sink.all(), "buffered, not written yet")
	h.Close()
	assert.Len(t, sink.all(), 1)
}

func TestFlushHelperOnLogger(t *testing.T) {
	log, _, sink, _ := newTest(t, Options{BatchSize: 1000, Interval: time.Hour})
	log.With("x", 1).Info("e", "run_id", 1)
	Flush(log.With("y", 2))
	assert.Len(t, sink.all(), 1)
	Flush(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))) // a logger without a journal is fine
}

func TestOrderKeptAcrossFlushes(t *testing.T) {
	log, h, sink, _ := newTest(t, Options{BatchSize: 1000, Interval: time.Hour})
	for i := 0; i < 6; i++ {
		log.Info("e", "run_id", 1, "i", i)
		if i%2 == 1 {
			require.NoError(t, h.Flush(context.Background()))
		}
	}
	ev := sink.all()
	require.Len(t, ev, 6)
	for i, e := range ev {
		assert.EqualValues(t, i, attrsOf(t, e)["i"])
	}
}

func TestPerRunCapDropsOldestWithOneWarning(t *testing.T) {
	log, h, sink, _ := newTest(t, Options{MaxPerRun: 3, BatchSize: 1000, Interval: time.Hour})
	for i := 0; i < 5; i++ {
		log.Info("a", "run_id", 1, "i", i)
	}
	log.Info("b", "run_id", 2) // another run is not affected
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	var run1 []store.RunEvent
	var warns int
	for _, e := range ev {
		if e.RunID == 1 {
			run1 = append(run1, e)
			if e.Level == "warn" {
				warns++
				assert.EqualValues(t, 2, attrsOf(t, e)["dropped"])
			}
		}
	}
	assert.Equal(t, 1, warns)
	require.Len(t, run1, 4, "the 3 newest and the warning")
	assert.EqualValues(t, 2, attrsOf(t, run1[0])["i"], "oldest two dropped")
	assert.Len(t, ev, 5)
}

func TestWriteErrorIsLoggedOnceAndRetried(t *testing.T) {
	log, h, sink, out := newTest(t, Options{Interval: time.Hour})
	sink.setErr(errors.New("db down"))
	log.Info("e", "run_id", 1)
	before := bytes.Count(out.Bytes(), []byte("\n"))
	require.Error(t, h.Flush(context.Background()))
	assert.Equal(t, before+1, bytes.Count(out.Bytes(), []byte("\n")), "one error line per flush")
	assert.Contains(t, out.String(), "writing events failed")
	assert.Empty(t, sink.all())

	sink.setErr(nil)
	require.NoError(t, h.Flush(context.Background()))
	assert.Len(t, sink.all(), 1, "the events were kept and written once the database is back")
}

func TestNoSinkKeepsEvents(t *testing.T) {
	out := &bytes.Buffer{}
	h := New(slog.NewJSONHandler(out, nil), Options{Interval: time.Hour})
	t.Cleanup(h.Close)
	slog.New(h).Info("e", "run_id", 1)
	require.NoError(t, h.Flush(context.Background()))
	sink := &fakeSink{}
	h.SetSink(sink)
	require.NoError(t, h.Flush(context.Background()))
	assert.Len(t, sink.all(), 1)
}

func TestRecordTimeAndMessage(t *testing.T) {
	log, h, sink, _ := newTest(t, Options{Interval: time.Hour})
	before := time.Now()
	log.Info("hello", "run_id", 1)
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 1)
	assert.Equal(t, "hello", ev[0].Message)
	assert.False(t, ev[0].TS.Before(before.Add(-time.Second)))
}
