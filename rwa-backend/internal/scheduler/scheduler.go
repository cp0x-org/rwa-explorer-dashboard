// Package scheduler runs collection jobs: on a cron schedule (UTC) or a fixed interval, once at start when the data
// are stale, and on manual requests polled from the run_requests table. A job is Collect, then Publish, Logos and
// Retention. Jobs never overlap inside the process (the database-level guard is the collector's advisory lock).
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/collect"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Defaults of Config.
const (
	DefaultPollInterval    = 30 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
	DefaultRunTimeout      = 2 * time.Hour
	// StaleAfterCron is how old the last successful run must be for COLLECT_ON_START to run when the schedule is a cron.
	StaleAfterCron = 24 * time.Hour
)

// Run triggers, as stored in collection_runs.trigger.
const (
	TriggerSchedule = "schedule"
	TriggerStartup  = "startup"
	TriggerManual   = "manual"
)

// Config is the scheduler configuration.
type Config struct {
	Cron          string        // COLLECT_CRON, UTC; used when Interval is zero. A CRON_TZ=/TZ= prefix is rejected
	Interval      time.Duration // COLLECT_INTERVAL; takes precedence over Cron
	OnStart       bool          // COLLECT_ON_START
	RetentionDays int           // SNAPSHOT_RETENTION_DAYS; <= 0 disables the retention step

	EventsRetentionDays int // RUN_EVENTS_RETENTION_DAYS; <= 0 disables the run journal retention

	RunTimeout      time.Duration // deadline of one job (RUN_TIMEOUT); default 2h
	PollInterval    time.Duration // run_requests poll period; default 30s
	ShutdownTimeout time.Duration // how long Run waits for the current job after ctx is cancelled; default 30s
}

// CollectResult is what a collection run reports.
type CollectResult struct {
	RunID       int64
	Status      string // succeeded | partial | failed
	NothingToDo bool
}

// Deps are the scheduler's collaborators. Collect and Publish are required; the others are optional (nil skips the step).
type Deps struct {
	// Collect runs one collection (mode refresh|add) for symbols (nil = all enabled). When a run row exists it must
	// return its RunID also together with an error. collect.ErrRunInProgress and collect.ErrInvalidOptions are
	// recognised with errors.Is.
	Collect func(ctx context.Context, mode string, symbols []string, trigger string) (CollectResult, error)
	// Publish builds and publishes the export of a finished run.
	Publish func(ctx context.Context, runID int64) error
	// Rebuild recomputes the export from stored snapshots without collecting (run_requests with mode rebuild).
	Rebuild func(ctx context.Context) (runID int64, err error)
	// MarkPublishFailed records on the run that its publish failed (store.MarkRunPublishFailed).
	MarkPublishFailed func(ctx context.Context, runID int64, cause error) error
	// Logos downloads missing logos; trigger is the trigger of the job, recorded on the rebuild it may start.
	Logos func(ctx context.Context, trigger string) error
	// Retention deletes snapshots fetched before cutoff.
	Retention func(ctx context.Context, cutoff time.Time) (int64, error)
	// EventsRetention deletes run journal events older than cutoff, except those of the current build's run.
	EventsRetention func(ctx context.Context, cutoff time.Time) (int64, error)
	// TakeRequest claims the oldest pending run request; it returns store.ErrNotFound (or nil, nil) when none is pending.
	TakeRequest func(ctx context.Context) (*store.RunRequest, error)
	// AttachRun links the run that served a request.
	AttachRun func(ctx context.Context, requestID, runID int64) error
	// ReleaseRequest gives a taken request back to the queue (retried on a later poll); reason is informational.
	ReleaseRequest func(ctx context.Context, requestID int64, reason string) error
	// FinishRequest records the final outcome of a request: store.OutcomeDone, OutcomeNothingToDo or OutcomeRejected.
	FinishRequest func(ctx context.Context, requestID int64, outcome string, err error) error
	// LastRun returns the start of the newest succeeded|partial run; ok is false when there is none.
	LastRun func(ctx context.Context) (startedAt time.Time, ok bool, err error)
	// Now is the clock (default time.Now).
	Now func() time.Time
}

// Scheduler runs jobs. Create it with New and call Run once.
type Scheduler struct {
	cfg  Config
	deps Deps
	log  *slog.Logger

	next func(time.Time) time.Time // next scheduled fire time after t
	mu   sync.Mutex                // held while a job runs
	wg   sync.WaitGroup
}

type job struct {
	mode      string
	symbols   []string
	trigger   string
	requestID int64 // 0 unless the job serves a run request
}

// New validates the configuration and returns a Scheduler.
func New(cfg Config, deps Deps, log *slog.Logger) (*Scheduler, error) {
	if deps.Collect == nil || deps.Publish == nil {
		return nil, errors.New("scheduler: Collect and Publish are required")
	}
	if log == nil {
		log = slog.Default()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = DefaultRunTimeout
	}
	s := &Scheduler{cfg: cfg, deps: deps, log: log}
	switch {
	case cfg.Interval > 0:
		s.next = func(t time.Time) time.Time { return t.Add(cfg.Interval) }
	case cfg.Cron != "":
		if spec := strings.TrimSpace(cfg.Cron); strings.HasPrefix(spec, "CRON_TZ=") || strings.HasPrefix(spec, "TZ=") {
			return nil, fmt.Errorf("scheduler: COLLECT_CRON %q: a CRON_TZ=/TZ= prefix is not supported, the schedule is always UTC", cfg.Cron)
		}
		sched, err := cron.ParseStandard(cfg.Cron)
		if err != nil {
			return nil, fmt.Errorf("scheduler: COLLECT_CRON %q: %w", cfg.Cron, err)
		}
		if ss, ok := sched.(*cron.SpecSchedule); ok {
			ss.Location = time.UTC // equivalent of cron.WithLocation(time.UTC)
		}
		s.next = func(t time.Time) time.Time { return sched.Next(t.UTC()) }
		if s.next(s.deps.Now()).IsZero() {
			return nil, fmt.Errorf("scheduler: COLLECT_CRON %q never fires", cfg.Cron)
		}
	default:
		return nil, errors.New("scheduler: either an interval or a cron schedule is required")
	}
	return s, nil
}

// staleAfter is the age of the last successful run after which COLLECT_ON_START runs.
func (s *Scheduler) staleAfter() time.Duration {
	if s.cfg.Interval > 0 {
		return s.cfg.Interval
	}
	return StaleAfterCron
}

// Run blocks until ctx is cancelled, then waits up to ShutdownTimeout for the current job (which honours ctx) and
// returns nil, or an error when the job did not finish in time.
func (s *Scheduler) Run(ctx context.Context) error {
	s.log.Info("scheduler started", "interval", s.cfg.Interval, "cron", s.cfg.Cron, "on_start", s.cfg.OnStart)
	if s.cfg.OnStart {
		s.onStart(ctx)
	}
	fireAt := s.next(s.deps.Now())
	timer := time.NewTimer(max(fireAt.Sub(s.deps.Now()), 0))
	defer timer.Stop()
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return s.shutdown()
		case <-timer.C:
			fired := fireAt
			fireAt = s.next(maxTime(fired, s.deps.Now()))
			timer.Reset(max(fireAt.Sub(s.deps.Now()), 0))
			if !s.tryStart(ctx, job{mode: "refresh", trigger: TriggerSchedule}) {
				s.log.Warn("scheduled run skipped: the previous job is still running", "scheduled_for", fired)
			}
		case <-poll.C:
			s.pollRequests(ctx)
		}
	}
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (s *Scheduler) onStart(ctx context.Context) {
	if s.deps.LastRun != nil {
		last, ok, err := s.deps.LastRun(ctx)
		if err != nil {
			s.log.Error("on-start check: last run lookup failed, running anyway", "err", err)
		} else if ok && s.deps.Now().Sub(last) < s.staleAfter() {
			s.log.Info("on-start run not needed: the last successful run is recent", "last_run_at", last, "stale_after", s.staleAfter())
			return
		}
	}
	if !s.tryStart(ctx, job{mode: "refresh", trigger: TriggerStartup}) {
		s.log.Warn("on-start run skipped: a job is running")
	}
}

// pollRequests claims one pending run request when no job is running and starts it. The scheduler's own mutex is
// the first guard (a request is only claimed while no job runs in this process); when another process holds the
// collector lock, the job releases the request again (see finishRequest).
func (s *Scheduler) pollRequests(ctx context.Context) {
	if s.deps.TakeRequest == nil || ctx.Err() != nil || !s.mu.TryLock() {
		return
	}
	req, err := s.deps.TakeRequest(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("poll run requests failed", "err", err)
	}
	if err != nil || req == nil {
		s.mu.Unlock()
		return
	}
	s.log.Info("run request taken", "request_id", req.ID, "mode", req.Mode, "symbols", req.Symbols, "requested_by", req.RequestedBy)
	s.spawn(ctx, job{mode: req.Mode, symbols: req.Symbols, trigger: TriggerManual, requestID: req.ID})
}

// tryStart starts j unless a job is running.
func (s *Scheduler) tryStart(ctx context.Context, j job) bool {
	if !s.mu.TryLock() {
		return false
	}
	s.spawn(ctx, j)
	return true
}

// spawn runs j in a goroutine; the caller holds s.mu, which the goroutine releases.
func (s *Scheduler) spawn(ctx context.Context, j job) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.mu.Unlock()
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("job panicked", "panic", fmt.Sprint(p), "trigger", j.trigger)
			}
		}()
		s.runJob(ctx, j)
	}()
}

func (s *Scheduler) shutdown() error {
	s.log.Info("scheduler stopping: waiting for the current job", "timeout", s.cfg.ShutdownTimeout)
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		s.log.Info("scheduler stopped")
		return nil
	case <-time.After(s.cfg.ShutdownTimeout):
		return fmt.Errorf("scheduler: the current job did not stop within %s", s.cfg.ShutdownTimeout)
	}
}

// outcome is how a job ended for the run request that started it.
type outcome struct {
	runID   int64
	kind    string // store.Outcome*; empty with release set
	err     error  // recorded on the request; for a release the reason
	release bool   // give the request back to the queue
}

// runJob executes one job under the RunTimeout deadline: Collect, then (unless there was nothing to do or the run
// failed) Publish, Logos, Retention. Retention runs only after a successful Publish; a Logos error does not stop it.
// A job started for a run request always ends with the request finished or released.
func (s *Scheduler) runJob(ctx context.Context, j job) {
	log := s.log.With("trigger", j.trigger, "mode", j.mode, "step", "scheduler")
	start := s.deps.Now()
	log.Info("job started", "symbols", j.symbols, "timeout", s.cfg.RunTimeout)

	jctx, cancel := context.WithTimeout(ctx, s.cfg.RunTimeout)
	defer cancel()
	var out outcome
	func() {
		// A panic must not leave a taken request behind: it is released, then the panic goes on to spawn's recover.
		defer func() {
			if p := recover(); p != nil {
				s.finishRequest(j, outcome{release: true, err: fmt.Errorf("job panicked: %v", p)}, log)
				panic(p)
			}
		}()
		if j.mode == "rebuild" {
			out = s.rebuild(jctx, j, log)
		} else {
			out = s.collectPublish(jctx, ctx, j, log)
		}
	}()
	s.attach(j, out.runID, log)
	s.finishRequest(j, out, log)
	log.Info("job ended", "run_id", out.runID, "duration", s.deps.Now().Sub(start))
	if j.mode == "rebuild" {
		runlog.Flush(s.log) // the rebuild run closes its own journal
	} else {
		runlog.CloseRun(s.log, out.runID) // the last event of the run: clients stop polling after it
	}
}

func (s *Scheduler) collectPublish(ctx, parent context.Context, j job, log *slog.Logger) outcome {
	res, err := s.deps.Collect(ctx, j.mode, j.symbols, j.trigger)
	out := outcome{runID: res.RunID, kind: store.OutcomeDone}
	switch {
	case errors.Is(err, collect.ErrRunInProgress):
		log.Warn("collect skipped: another collector run is in progress")
		return outcome{release: true, err: err}
	case errors.Is(err, collect.ErrInvalidOptions):
		log.Error("run request rejected", "err", err)
		return outcome{kind: store.OutcomeRejected, err: err}
	case err != nil && parent.Err() != nil:
		// Shutdown: the run (if any) was recorded as interrupted; the request is served again after the restart.
		log.Warn("collect interrupted by shutdown", "run_id", res.RunID, "err", err)
		out.release, out.err = true, err
		return out
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		log.Error("run timed out", "run_id", res.RunID, "timeout", s.cfg.RunTimeout)
		out.err = err
		return out
	case err != nil && res.RunID == 0:
		// Failed before the run row existed (database error, ...): nothing was done, try again on a later poll.
		log.Error("collect failed before the run started", "err", err)
		return outcome{release: true, err: err}
	case err != nil:
		log.Error("collect failed", "run_id", res.RunID, "err", err)
		out.err = err
		return out
	case res.NothingToDo:
		log.Info("nothing to collect", "run_id", res.RunID)
		out.kind = store.OutcomeNothingToDo
		return out
	case res.Status == "failed":
		log.Warn("run failed, nothing to publish", "run_id", res.RunID)
		return out
	}
	log.Info("collection run ended, publishing", "run_id", res.RunID, "status", res.Status)
	// Everything below belongs to the run: its context carries the id for callbacks that log without a run attribute.
	ctx = runlog.WithRun(ctx, res.RunID)
	if err := s.deps.Publish(ctx, res.RunID); err != nil {
		// Warn: the publish step has logged the error event of this failure in the run's journal.
		log.Warn("publish failed, the previous build stays current; skipping logos and retention", "run_id", res.RunID, "err", err)
		if s.deps.MarkPublishFailed != nil {
			if merr := s.deps.MarkPublishFailed(ctx, res.RunID, err); merr != nil {
				log.Error("recording the publish failure on the run failed", "run_id", res.RunID, "err", merr)
			}
		}
		out.err = fmt.Errorf("publish: %w", err)
		return out
	}
	if s.deps.Logos != nil {
		if err := s.deps.Logos(ctx, j.trigger); err != nil {
			log.Error("logos step failed", "run_id", res.RunID, "err", err)
		}
	}
	rlog := s.log.With("run_id", res.RunID, "step", "retention")
	if s.deps.Retention != nil && s.cfg.RetentionDays > 0 {
		cutoff := s.deps.Now().AddDate(0, 0, -s.cfg.RetentionDays)
		n, err := s.deps.Retention(ctx, cutoff)
		if err != nil {
			rlog.Error("snapshot retention failed", "err", err)
		} else {
			rlog.Info("snapshot retention done", "deleted_snapshots", n, "cutoff", cutoff)
		}
	}
	if s.deps.EventsRetention != nil && s.cfg.EventsRetentionDays > 0 {
		cutoff := s.deps.Now().AddDate(0, 0, -s.cfg.EventsRetentionDays)
		n, err := s.deps.EventsRetention(ctx, cutoff)
		if err != nil {
			rlog.Error("run events retention failed", "err", err)
		} else {
			rlog.Info("run events retention done", "deleted_events", n, "cutoff", cutoff)
		}
	}
	log.Info("job finished", "run_id", res.RunID, "status", res.Status)
	return out
}

func (s *Scheduler) rebuild(ctx context.Context, j job, log *slog.Logger) outcome {
	if s.deps.Rebuild == nil {
		log.Error("rebuild requested but not supported by this collector", "request_id", j.requestID)
		return outcome{kind: store.OutcomeRejected, err: errors.New("rebuild is not supported by this collector")}
	}
	runID, err := s.deps.Rebuild(ctx)
	if err != nil {
		log.Warn("rebuild failed", "run_id", runID, "err", err)
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return outcome{runID: runID, release: true, err: err}
		}
		return outcome{runID: runID, kind: store.OutcomeDone, err: err}
	}
	log.Info("rebuild finished", "run_id", runID)
	runlog.Flush(s.log)
	return outcome{runID: runID, kind: store.OutcomeDone}
}

// attach links the run to the request that started the job. It uses a context that survives shutdown so the link is
// still written when the run was interrupted.
func (s *Scheduler) attach(j job, runID int64, log *slog.Logger) {
	if j.requestID == 0 || runID == 0 || s.deps.AttachRun == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.deps.AttachRun(ctx, j.requestID, runID); err != nil {
		log.Error("attach run to request failed", "request_id", j.requestID, "run_id", runID, "err", err)
	}
}

// finishRequest ends the request of a job: finished with its outcome, or released back to the queue. Like attach it
// ignores shutdown.
func (s *Scheduler) finishRequest(j job, out outcome, log *slog.Logger) {
	if j.requestID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	switch {
	case out.release && s.deps.ReleaseRequest != nil:
		reason := ""
		if out.err != nil {
			reason = out.err.Error()
		}
		err = s.deps.ReleaseRequest(ctx, j.requestID, reason)
	case !out.release && s.deps.FinishRequest != nil:
		err = s.deps.FinishRequest(ctx, j.requestID, out.kind, out.err)
	}
	if err != nil {
		log.Error("recording the request outcome failed", "request_id", j.requestID, "err", err)
	}
}
