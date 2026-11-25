package runlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// poisonSink rejects every batch that holds a row with message "poison" like Postgres rejects a bad row.
type poisonSink struct {
	mu     sync.Mutex
	stored []store.RunEvent
	calls  int
	err    error // returned for every call when set
}

func (p *poisonSink) InsertRunEvents(_ context.Context, rows []store.RunEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return p.err
	}
	for _, r := range rows {
		if r.Message == "poison" {
			return fmt.Errorf("store: insert: %w", &pgconn.PgError{Code: "22021", Message: "invalid byte sequence"})
		}
	}
	p.stored = append(p.stored, rows...)
	return nil
}

func (p *poisonSink) messages() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, r := range p.stored {
		out = append(out, r.Message)
	}
	return out
}

func newBuf() *lockedWriter { return &lockedWriter{w: &bytes.Buffer{}} }

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.String()
}

func TestNULIsStripped(t *testing.T) {
	l, h, sink, _ := newTest(t, Options{})
	l.Info("bad\x00message", "run_id", 1, "s", "a\x00b", "k\x00ey", "v",
		"list", []string{"x\x00y"}, "m", map[string]any{"a\x00": "b\x00"}, "e", errors.New("boom\x00"), "any", "z\x00")
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 1)
	assert.Equal(t, "badmessage", ev[0].Message)
	assert.NotContains(t, string(ev[0].Attrs), `\u0000`)
	assert.True(t, json.Valid(ev[0].Attrs))
	a := attrsOf(t, ev[0])
	assert.Equal(t, "ab", a["s"])
	assert.Equal(t, "v", a["key"])
	assert.Equal(t, []any{"xy"}, a["list"])
	assert.Equal(t, map[string]any{"a": "b"}, a["m"])
	assert.Equal(t, "boom", a["e"])
}

func TestNaNAndInfKeepOtherAttrs(t *testing.T) {
	l, h, sink, _ := newTest(t, Options{})
	l.Info("m", "run_id", 1, "n", math.NaN(), "p", math.Inf(1), "ok", 1, "bad", []float64{math.NaN()}, "fn", func() {})
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 1)
	a := attrsOf(t, ev[0])
	assert.Equal(t, "NaN", a["n"])
	assert.Equal(t, "+Inf", a["p"])
	assert.EqualValues(t, 1, a["ok"])
	assert.NotNil(t, a["bad"], "an unencodable value falls back to its text")
}

func TestRunIDInGroupIsNotTheRunID(t *testing.T) {
	l, h, sink, _ := newTest(t, Options{})
	g := l.WithGroup("req")
	g.Info("in group", "run_id", 9, "step", "evil")
	g.With("run_id", 8).Info("in group with attrs")
	l.With("run_id", 1).WithGroup("req").Info("owned", "run_id", 9, "step", "evil")
	require.NoError(t, h.Flush(context.Background()))
	ev := sink.all()
	require.Len(t, ev, 1, "only the record with a real run id is stored")
	assert.EqualValues(t, 1, ev[0].RunID)
	assert.Equal(t, "run", ev[0].Step)
	a := attrsOf(t, ev[0])
	assert.EqualValues(t, 9, a["req.run_id"])
	assert.Equal(t, "evil", a["req.step"])
}

func TestPoisonRowDoesNotBlockTheBatch(t *testing.T) {
	sink := &poisonSink{}
	buf := newBuf()
	h := New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}), Options{})
	h.SetSink(sink)
	t.Cleanup(h.Close)
	l := slog.New(h)
	l.Info("a", "run_id", 5)
	l.Info("poison", "run_id", 5)
	l.Info("c", "run_id", 5)
	require.NoError(t, h.Flush(context.Background()))
	assert.Equal(t, []string{"a", "c"}, sink.messages())
	assert.Contains(t, buf.String(), "dropped an event the database rejects")
	assert.Equal(t, 1, strings.Count(buf.String(), "dropped an event"))
	// nothing is left to retry
	sink.mu.Lock()
	calls := sink.calls
	sink.mu.Unlock()
	require.NoError(t, h.Flush(context.Background()))
	sink.mu.Lock()
	assert.Equal(t, calls, sink.calls)
	sink.mu.Unlock()
}

func TestRetryIsCapped(t *testing.T) {
	sink := &poisonSink{err: errors.New("connection refused")}
	buf := newBuf()
	h := New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}), Options{MaxAttempts: 3, Interval: time.Hour})
	h.SetSink(sink)
	t.Cleanup(h.Close)
	l := slog.New(h)
	l.Info("a", "run_id", 5)
	l.Info("b", "run_id", 6)

	for i := 0; i < 2; i++ {
		require.Error(t, h.Flush(context.Background()))
		h.c.mu.Lock()
		assert.Len(t, h.c.buf, 2, "requeued at attempt %d", i+1)
		h.c.mu.Unlock()
	}
	require.Error(t, h.Flush(context.Background())) // third failure: dropped
	h.c.mu.Lock()
	require.Len(t, h.c.buf, 2, "one warn event per run replaces the dropped batch")
	assert.Equal(t, "warn", h.c.buf[0].Level)
	assert.Contains(t, h.c.buf[0].Message, "dropped")
	h.c.mu.Unlock()
	assert.Equal(t, 1, strings.Count(buf.String(), "giving up on a batch"))

	sink.mu.Lock()
	sink.err = nil
	sink.mu.Unlock()
	require.NoError(t, h.Flush(context.Background()))
	assert.Len(t, sink.messages(), 2, "the warn events are stored; a and b are gone")
	for _, m := range sink.messages() {
		assert.Contains(t, m, "dropped")
	}
}

func TestRequeueKeepsOrderUntilSuccess(t *testing.T) {
	sink := &poisonSink{err: errors.New("down")}
	h := New(slog.NewJSONHandler(newBuf(), &slog.HandlerOptions{Level: slog.LevelInfo}), Options{Interval: time.Hour})
	h.SetSink(sink)
	t.Cleanup(h.Close)
	l := slog.New(h)
	l.Info("1", "run_id", 1)
	require.Error(t, h.Flush(context.Background()))
	l.Info("2", "run_id", 1)
	sink.mu.Lock()
	sink.err = nil
	sink.mu.Unlock()
	require.NoError(t, h.Flush(context.Background()))
	assert.Equal(t, []string{"1", "2"}, sink.messages())
}

func TestConcurrentUse(t *testing.T) {
	sink := &poisonSink{}
	h := New(slog.NewJSONHandler(newBuf(), &slog.HandlerOptions{Level: slog.LevelInfo}), Options{BatchSize: 10, Interval: 5 * time.Millisecond})
	h.SetSink(sink)
	l := slog.New(h)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ll := l.With("run_id", g%4+1).WithGroup("grp")
			for i := 0; i < 100; i++ {
				ll.Info("event", "i", i, "n", math.NaN(), "s", "a\x00b")
				l.With("step", "x").WithGroup("q").Warn("w", "run_id", 3)
				if i%10 == 0 {
					_ = h.Flush(context.Background())
				}
			}
		}()
	}
	wg.Add(2)
	go func() { defer wg.Done(); h.Close() }()
	go func() { defer wg.Done(); h.Close() }()
	wg.Wait()
	h.Close()
	assert.NotEmpty(t, sink.messages())
}

func TestCloseRunWritesTerminalEvent(t *testing.T) {
	l, _, sink, _ := newTest(t, Options{})
	CloseRun(l, 4)
	CloseRun(l, 0)
	ev := sink.all()
	require.Len(t, ev, 1)
	assert.EqualValues(t, 4, ev[0].RunID)
	assert.Equal(t, "run", ev[0].Step)
	assert.Equal(t, MsgClosed, ev[0].Message)
	assert.Equal(t, "info", ev[0].Level)
}
