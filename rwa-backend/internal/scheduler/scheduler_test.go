package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

var quiet = slog.New(slog.DiscardHandler)

type finish struct {
	id      int64
	outcome string
	err     error
}

type call struct {
	mode    string
	symbols []string
	trigger string
}

// fake records calls and implements the deps.
type fake struct {
	mu           sync.Mutex
	collects     []call
	steps        []string
	attached     [][2]int64
	requests     []*store.RunRequest
	collect      func(ctx context.Context) (CollectResult, error)
	pubErr       error
	logosTrigger string
	released     []int64
	finished     []finish
	marked       []int64
	last         func() (time.Time, bool, error)
	now          time.Time
}

func (f *fake) deps() Deps {
	d := Deps{
		Collect: func(ctx context.Context, mode string, symbols []string, trigger string) (CollectResult, error) {
			f.mu.Lock()
			f.collects = append(f.collects, call{mode, symbols, trigger})
			f.steps = append(f.steps, "collect")
			f.mu.Unlock()
			if f.collect != nil {
				return f.collect(ctx)
			}
			return CollectResult{RunID: 7, Status: "succeeded"}, nil
		},
		Publish: func(context.Context, int64) error { f.step("publish"); return f.pubErr },
		Logos: func(_ context.Context, trigger string) error {
			f.step("logos")
			f.mu.Lock()
			f.logosTrigger = trigger
			f.mu.Unlock()
			return nil
		},
		Retention: func(_ context.Context, _ time.Time) (int64, error) {
			f.step("retention")
			return 0, nil
		},
		TakeRequest: func(context.Context) (*store.RunRequest, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.requests) == 0 {
				return nil, store.ErrNotFound
			}
			r := f.requests[0]
			f.requests = f.requests[1:]
			return r, nil
		},
		MarkPublishFailed: func(_ context.Context, runID int64, _ error) error {
			f.mu.Lock()
			f.marked = append(f.marked, runID)
			f.mu.Unlock()
			return nil
		},
		ReleaseRequest: func(_ context.Context, id int64, _ string) error {
			f.mu.Lock()
			f.released = append(f.released, id)
			f.mu.Unlock()
			return nil
		},
		FinishRequest: func(_ context.Context, id int64, outcome string, err error) error {
			f.mu.Lock()
			f.finished = append(f.finished, finish{id, outcome, err})
			f.mu.Unlock()
			return nil
		},
		AttachRun: func(_ context.Context, reqID, runID int64) error {
			f.mu.Lock()
			f.attached = append(f.attached, [2]int64{reqID, runID})
			f.mu.Unlock()
			return nil
		},
	}
	if f.last != nil {
		d.LastRun = func(context.Context) (time.Time, bool, error) { return f.last() }
	}
	if !f.now.IsZero() {
		d.Now = func() time.Time { return f.now }
	}
	return d
}

func (f *fake) step(s string) { f.mu.Lock(); f.steps = append(f.steps, s); f.mu.Unlock() }

func (f *fake) snapshot() (calls []call, steps []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.collects...), append([]string(nil), f.steps...)
}

// start runs the scheduler and returns a stop func that cancels and returns Run's error.
func start(t *testing.T, cfg Config, f *fake) (stop func() error) {
	t.Helper()
	s, err := New(cfg, f.deps(), quiet)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return")
			return nil
		}
	}
}

func TestNewValidation(t *testing.T) {
	_, err := New(Config{}, Deps{Collect: nil, Publish: nil}, quiet)
	require.Error(t, err)
	d := (&fake{}).deps()
	_, err = New(Config{}, d, quiet)
	require.ErrorContains(t, err, "interval or a cron")
	_, err = New(Config{Cron: "not a cron"}, d, quiet)
	require.ErrorContains(t, err, "COLLECT_CRON")
	_, err = New(Config{Cron: "0 6 * * *"}, d, quiet)
	require.NoError(t, err)
	_, err = New(Config{Cron: "CRON_TZ=Europe/Moscow 0 6 * * *"}, d, quiet)
	require.ErrorContains(t, err, "CRON_TZ")
	_, err = New(Config{Cron: "TZ=UTC 0 6 * * *"}, d, quiet)
	require.ErrorContains(t, err, "not supported")
	_, err = New(Config{Cron: "0 0 31 2 *"}, d, quiet) // 31 February
	require.ErrorContains(t, err, "never fires")
	// Interval wins over cron: exclusivity itself is a config error (config.ValidateCollector).
	s, err := New(Config{Cron: "0 6 * * *", Interval: time.Hour}, d, quiet)
	require.NoError(t, err)
	t0 := time.Date(2026, 6, 10, 5, 30, 0, 0, time.UTC)
	require.Equal(t, t0.Add(time.Hour), s.next(t0))
}

func TestCronNextIsUTC(t *testing.T) {
	s, err := New(Config{Cron: "0 6 * * *"}, (&fake{}).deps(), quiet)
	require.NoError(t, err)
	moscow := time.FixedZone("MSK", 3*3600)
	// 08:00 MSK is 05:00 UTC: the next 06:00 UTC is the same day.
	got := s.next(time.Date(2026, 6, 10, 8, 0, 0, 0, moscow))
	require.Equal(t, time.Date(2026, 6, 10, 6, 0, 0, 0, time.UTC), got)
	got = s.next(time.Date(2026, 6, 10, 6, 0, 0, 0, time.UTC))
	require.Equal(t, time.Date(2026, 6, 11, 6, 0, 0, 0, time.UTC), got)
}

func TestIntervalRunsFullJob(t *testing.T) {
	f := &fake{}
	stop := start(t, Config{Interval: 15 * time.Millisecond, RetentionDays: 90}, f)
	require.Eventually(t, func() bool { c, _ := f.snapshot(); return len(c) >= 2 }, 3*time.Second, 5*time.Millisecond)
	require.NoError(t, stop())
	calls, steps := f.snapshot()
	require.Equal(t, call{"refresh", nil, TriggerSchedule}, calls[0])
	require.Equal(t, []string{"collect", "publish", "logos", "retention"}, steps[:4])
}

func TestOnStartLogic(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		cfg  Config
		last func() (time.Time, bool, error)
		runs bool
	}{
		{"interval, stale", Config{Interval: time.Hour, OnStart: true}, func() (time.Time, bool, error) { return now.Add(-2 * time.Hour), true, nil }, true},
		{"interval, fresh", Config{Interval: time.Hour, OnStart: true}, func() (time.Time, bool, error) { return now.Add(-30 * time.Minute), true, nil }, false},
		{"cron, older than 24h", Config{Cron: "0 0 1 1 *", OnStart: true}, func() (time.Time, bool, error) { return now.Add(-25 * time.Hour), true, nil }, true},
		{"cron, 23h old", Config{Cron: "0 0 1 1 *", OnStart: true}, func() (time.Time, bool, error) { return now.Add(-23 * time.Hour), true, nil }, false},
		{"no run yet", Config{Cron: "0 0 1 1 *", OnStart: true}, func() (time.Time, bool, error) { return time.Time{}, false, nil }, true},
		{"lookup error runs anyway", Config{Cron: "0 0 1 1 *", OnStart: true}, func() (time.Time, bool, error) { return time.Time{}, false, errors.New("db down") }, true},
		{"disabled", Config{Cron: "0 0 1 1 *", OnStart: false}, func() (time.Time, bool, error) { return time.Time{}, false, nil }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{now: now, last: c.last}
			cfg := c.cfg
			if cfg.Interval > 0 {
				cfg.Interval = time.Hour // fake clock: the interval timer never fires
			}
			stop := start(t, cfg, f)
			if c.runs {
				require.Eventually(t, func() bool { calls, _ := f.snapshot(); return len(calls) == 1 }, 2*time.Second, 5*time.Millisecond)
			} else {
				time.Sleep(100 * time.Millisecond)
			}
			require.NoError(t, stop())
			calls, _ := f.snapshot()
			if c.runs {
				require.Equal(t, call{"refresh", nil, TriggerStartup}, calls[0])
			} else {
				require.Empty(t, calls)
			}
		})
	}
}

func TestRequestPollingUsesRequestModeAndSymbols(t *testing.T) {
	f := &fake{requests: []*store.RunRequest{{ID: 42, Mode: "add", Symbols: []string{"NVDA", "AAPL"}, RequestedBy: "admin-api"}}}
	stop := start(t, Config{Cron: "0 0 1 1 *", PollInterval: 10 * time.Millisecond, RetentionDays: 90}, f)
	require.Eventually(t, func() bool { c, steps := f.snapshot(); return len(c) == 1 && len(steps) == 4 }, 3*time.Second, 5*time.Millisecond)
	require.NoError(t, stop())
	calls, _ := f.snapshot()
	require.Equal(t, call{"add", []string{"NVDA", "AAPL"}, TriggerManual}, calls[0])
	require.Equal(t, [][2]int64{{42, 7}}, f.attached)
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.finished) == 1 }, time.Second, time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(t, finish{42, store.OutcomeDone, nil}, f.finished[0])
	require.Equal(t, TriggerManual, f.logosTrigger, "the logos rebuild carries the trigger of the job")
}

func TestRebuildRequest(t *testing.T) {
	f := &fake{requests: []*store.RunRequest{{ID: 5, Mode: "rebuild"}}}
	d := f.deps()
	var rebuilt atomic.Int32
	d.Rebuild = func(context.Context) (int64, error) { rebuilt.Add(1); return 99, nil }
	s, err := New(Config{Cron: "0 0 1 1 *", PollInterval: 10 * time.Millisecond}, d, quiet)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	require.Eventually(t, func() bool { return rebuilt.Load() == 1 }, 3*time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	calls, _ := f.snapshot()
	require.Empty(t, calls, "a rebuild does not collect")
	require.Equal(t, [][2]int64{{5, 99}}, f.attached)
}

func TestStepPolicy(t *testing.T) {
	t.Run("publish failure skips logos and retention", func(t *testing.T) {
		f := &fake{pubErr: errors.New("boom")}
		stop := start(t, Config{Interval: time.Hour, OnStart: true, RetentionDays: 90}, f)
		require.Eventually(t, func() bool { _, s := f.snapshot(); return len(s) >= 2 }, 2*time.Second, 5*time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, stop())
		_, steps := f.snapshot()
		require.Equal(t, []string{"collect", "publish"}, steps)
	})
	t.Run("nothing to do and failed runs do not publish", func(t *testing.T) {
		for _, res := range []CollectResult{{RunID: 1, NothingToDo: true, Status: "succeeded"}, {RunID: 2, Status: "failed"}} {
			f := &fake{collect: func(context.Context) (CollectResult, error) { return res, nil }}
			stop := start(t, Config{Interval: time.Hour, OnStart: true, RetentionDays: 90}, f)
			require.Eventually(t, func() bool { _, s := f.snapshot(); return len(s) >= 1 }, 2*time.Second, 5*time.Millisecond)
			time.Sleep(50 * time.Millisecond)
			require.NoError(t, stop())
			_, steps := f.snapshot()
			require.Equal(t, []string{"collect"}, steps)
		}
	})
	t.Run("collect error", func(t *testing.T) {
		f := &fake{collect: func(context.Context) (CollectResult, error) { return CollectResult{}, errors.New("auth") }}
		stop := start(t, Config{Interval: time.Hour, OnStart: true}, f)
		require.Eventually(t, func() bool { _, s := f.snapshot(); return len(s) >= 1 }, 2*time.Second, 5*time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, stop())
		_, steps := f.snapshot()
		require.Equal(t, []string{"collect"}, steps)
	})
}

func TestJobsNeverOverlap(t *testing.T) {
	var running, maxRunning, started atomic.Int32
	release := make(chan struct{})
	f := &fake{collect: func(context.Context) (CollectResult, error) {
		n := running.Add(1)
		started.Add(1)
		if n > maxRunning.Load() {
			maxRunning.Store(n)
		}
		<-release
		running.Add(-1)
		return CollectResult{RunID: 1, Status: "succeeded"}, nil
	}}
	stop := start(t, Config{Interval: 5 * time.Millisecond, PollInterval: 5 * time.Millisecond}, f)
	require.Eventually(t, func() bool { return started.Load() == 1 }, 2*time.Second, time.Millisecond)
	time.Sleep(100 * time.Millisecond) // ~20 ticks fire and are skipped
	require.EqualValues(t, 1, started.Load())
	close(release)
	require.NoError(t, stop())
	require.EqualValues(t, 1, maxRunning.Load())
}

func TestShutdownWaitsForCurrentJob(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	f := &fake{collect: func(context.Context) (CollectResult, error) {
		close(entered)
		<-release
		return CollectResult{RunID: 1, Status: "succeeded"}, nil
	}}
	s, err := New(Config{Interval: time.Hour, OnStart: true, ShutdownTimeout: 5 * time.Second}, f.deps(), quiet)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-entered
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while the job was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	_, steps := f.snapshot()
	require.Equal(t, "collect", steps[0])
}

func TestShutdownTimeout(t *testing.T) {
	entered := make(chan struct{})
	block := make(chan struct{})
	defer close(block)
	f := &fake{collect: func(context.Context) (CollectResult, error) { close(entered); <-block; return CollectResult{}, nil }}
	s, err := New(Config{Interval: time.Hour, OnStart: true, ShutdownTimeout: 50 * time.Millisecond}, f.deps(), quiet)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-entered
	cancel()
	require.ErrorContains(t, <-done, "did not stop")
}
