package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/collect"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// runRequestJob serves one request through the scheduler and waits until it was finished or released.
func runRequestJob(t *testing.T, f *fake, cfg Config) {
	t.Helper()
	if cfg.Cron == "" && cfg.Interval == 0 {
		cfg.Cron = "0 0 1 1 *"
	}
	cfg.PollInterval = 5 * time.Millisecond
	stop := start(t, cfg, f)
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.finished)+len(f.released) > 0
	}, 3*time.Second, 2*time.Millisecond)
	require.NoError(t, stop())
}

func TestRequestOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		collect  func(ctx context.Context) (CollectResult, error)
		outcome  string // "" = released
		attached bool
		errPart  string
	}{
		{"done", func(context.Context) (CollectResult, error) { return CollectResult{RunID: 7, Status: "succeeded"}, nil }, store.OutcomeDone, true, ""},
		{"nothing to do", func(context.Context) (CollectResult, error) { return CollectResult{NothingToDo: true}, nil }, store.OutcomeNothingToDo, false, ""},
		{"invalid symbols are rejected", func(context.Context) (CollectResult, error) {
			return CollectResult{}, fmt.Errorf("%w: unknown or disabled symbols: NOPE", collect.ErrInvalidOptions)
		}, store.OutcomeRejected, false, "NOPE"},
		{"run in progress is released", func(context.Context) (CollectResult, error) { return CollectResult{}, collect.ErrRunInProgress }, "", false, "in progress"},
		{"database error before the run is released", func(context.Context) (CollectResult, error) { return CollectResult{}, errors.New("connection reset") }, "", false, "connection reset"},
		{"error with a run id still attaches the run", func(context.Context) (CollectResult, error) {
			return CollectResult{RunID: 9, Status: "failed"}, errors.New("collect: finish run 9: boom")
		}, store.OutcomeDone, true, "finish run"},
		{"failed run is done", func(context.Context) (CollectResult, error) { return CollectResult{RunID: 8, Status: "failed"}, nil }, store.OutcomeDone, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{requests: []*store.RunRequest{{ID: 11, Mode: "refresh"}}, collect: c.collect}
			runRequestJob(t, f, Config{})
			f.mu.Lock()
			defer f.mu.Unlock()
			if c.outcome == "" {
				require.Equal(t, []int64{11}, f.released)
				require.Empty(t, f.finished)
			} else {
				require.Empty(t, f.released)
				require.Len(t, f.finished, 1)
				require.Equal(t, c.outcome, f.finished[0].outcome)
				if c.errPart == "" {
					require.NoError(t, f.finished[0].err)
				} else {
					require.ErrorContains(t, f.finished[0].err, c.errPart)
				}
			}
			if c.attached {
				require.Len(t, f.attached, 1)
				require.EqualValues(t, 11, f.attached[0][0])
				require.NotZero(t, f.attached[0][1])
			} else {
				require.Empty(t, f.attached)
			}
		})
	}
}

func TestInterruptedRunIsAttachedAndRequestReleased(t *testing.T) {
	entered := make(chan struct{})
	f := &fake{requests: []*store.RunRequest{{ID: 3, Mode: "refresh"}}}
	f.collect = func(ctx context.Context) (CollectResult, error) {
		close(entered)
		<-ctx.Done()
		return CollectResult{RunID: 21, Status: "failed"}, ctx.Err()
	}
	s, err := New(Config{Cron: "0 0 1 1 *", PollInterval: 5 * time.Millisecond}, f.deps(), quiet)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-entered
	cancel()
	require.NoError(t, <-done)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(t, [][2]int64{{3, 21}}, f.attached, "the interrupted run is linked to its request")
	require.Equal(t, []int64{3}, f.released, "an interrupted request is served again after the restart")
	require.Empty(t, f.finished)
}

func TestRebuildRequestOutcomes(t *testing.T) {
	f := &fake{requests: []*store.RunRequest{{ID: 5, Mode: "rebuild"}}}
	runRequestJob(t, f, Config{}) // Rebuild is nil in the fake: not supported
	require.Equal(t, store.OutcomeRejected, f.finished[0].outcome)
}

func TestPublishFailureIsRecordedOnTheRun(t *testing.T) {
	f := &fake{pubErr: errors.New("boom")}
	stop := start(t, Config{Interval: time.Hour, OnStart: true, RetentionDays: 90}, f)
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.marked) == 1 }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, stop())
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(t, []int64{7}, f.marked)
}

func TestRunTimeoutAppliesToTheJob(t *testing.T) {
	f := &fake{}
	cause := make(chan error, 1)
	f.collect = func(ctx context.Context) (CollectResult, error) {
		<-ctx.Done()
		cause <- ctx.Err()
		return CollectResult{RunID: 4, Status: "failed"}, ctx.Err()
	}
	stop := start(t, Config{Interval: time.Hour, OnStart: true, RunTimeout: 30 * time.Millisecond}, f)
	select {
	case err := <-cause:
		require.ErrorIs(t, err, context.DeadlineExceeded, "the job context ends by deadline, not by shutdown")
	case <-time.After(3 * time.Second):
		t.Fatal("the job did not time out")
	}
	require.NoError(t, stop())
}

func TestPollSkipsWhileJobRuns(t *testing.T) {
	// A request is claimed only while no job runs in this process.
	release := make(chan struct{})
	started := make(chan struct{})
	f := &fake{requests: []*store.RunRequest{{ID: 1, Mode: "refresh"}}}
	f.collect = func(context.Context) (CollectResult, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return CollectResult{RunID: 2, Status: "succeeded"}, nil
	}
	stop := start(t, Config{Interval: time.Hour, OnStart: true, PollInterval: 5 * time.Millisecond}, f)
	<-started
	time.Sleep(60 * time.Millisecond)
	f.mu.Lock()
	require.Len(t, f.requests, 1, "the request stays queued while the startup job runs")
	f.mu.Unlock()
	close(release)
	require.NoError(t, stop())
}
