// Package runlog is the per-run journal: a slog.Handler that forwards every record to the process handler (stdout) and
// also stores the records that belong to a collection run in the run_events table, so the course of a run is visible
// through the API and not only in container logs.
//
// A record belongs to a run when it carries a run id: an attribute run_id (also one added with Logger.With) or a context
// value set by WithRun. The attribute step names the stage (collect.cmc, collect.portals, collect.underlying, publish,
// logos, retention, scheduler, run; default run). Events are buffered and written in batches through the pool, never on
// the run's own transaction; write errors are logged to the process handler and never fail the run.
package runlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Defaults of Options.
const (
	DefaultBatchSize = 200
	DefaultInterval  = 2 * time.Second
	DefaultMaxPerRun = 10_000
	// DefaultMaxAttempts is how many consecutive failed writes of a batch are tolerated (about a minute with the
	// default interval) before it is dropped.
	DefaultMaxAttempts = 30

	maxStringLen = 4000
	flushTimeout = 10 * time.Second
)

// Attribute keys the handler understands.
const (
	AttrRunID = "run_id"
	AttrStep  = "step"
)

// MsgClosed is the message of the terminal event of a run's journal (step run): the job that owns the run writes it
// last, and the API reports journal_open=false once it exists.
const MsgClosed = "journal closed"

// CloseRun writes the terminal event of run runID's journal and flushes it. Call it when the job that owns the run
// has nothing more to log.
func CloseRun(l *slog.Logger, runID int64) {
	if runID <= 0 {
		return
	}
	l.Info(MsgClosed, AttrRunID, runID, AttrStep, "run")
	Flush(l)
}

// Sink stores events; *store.Store implements it.
type Sink interface {
	InsertRunEvents(ctx context.Context, rows []store.RunEvent) error
}

// Options configures a Handler.
type Options struct {
	Level     slog.Level    // minimum level stored (RUN_EVENTS_LEVEL); the zero value is info
	BatchSize int           // flush when this many events are buffered; default 200
	Interval  time.Duration // flush at least this often; default 2 s
	MaxPerRun int           // buffered events per run before the oldest are dropped; default 10 000
	// MaxAttempts is the number of consecutive failed writes (other than poison rows) after which the batch is
	// dropped; default 30.
	MaxAttempts int
}

type ctxKey struct{}

// WithRun returns a context whose records belong to run runID.
func WithRun(ctx context.Context, runID int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, runID)
}

// RunID returns the run id set by WithRun, or 0.
func RunID(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKey{}).(int64)
	return id
}

// core is the state shared by a Handler and the handlers derived from it with WithAttrs and WithGroup.
type core struct {
	opts  Options
	inner slog.Handler // base process handler, used to report write errors
	sink  atomic.Pointer[Sink]

	mu      sync.Mutex // guards buf, perRun, dropped
	buf     []store.RunEvent
	perRun  map[int64]int
	dropped map[int64]int

	flushMu sync.Mutex // serializes writes so events keep their order
	fails   int        // consecutive failed writes; guarded by flushMu
	kick    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

// Handler is the slog.Handler described in the package comment.
type Handler struct {
	c      *core
	inner  slog.Handler // inner with this handler's attrs and groups applied
	runID  int64        // from WithAttrs(run_id)
	step   string       // from WithAttrs(step)
	pre    []slog.Attr  // attrs added with WithAttrs, group prefix applied to the key
	prefix string       // current group prefix, "a.b."
}

// New returns a Handler forwarding to inner. Events are written once SetSink is called. The background flusher runs until
// Close.
func New(inner slog.Handler, opts Options) *Handler {
	if opts.BatchSize <= 0 {
		opts.BatchSize = DefaultBatchSize
	}
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.MaxPerRun <= 0 {
		opts.MaxPerRun = DefaultMaxPerRun
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	c := &core{opts: opts, inner: inner, perRun: map[int64]int{}, dropped: map[int64]int{},
		kick: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go c.loop()
	return &Handler{c: c, inner: inner}
}

// SetSink sets where events are written. Records of runs logged before it are buffered.
func (h *Handler) SetSink(s Sink) { h.c.sink.Store(&s) }

// Flush writes the buffered events now.
func (h *Handler) Flush(ctx context.Context) error { return h.c.flush(ctx) }

// CloseTimeout bounds Close: a database that does not answer must not hold the process past its shutdown budget.
const CloseTimeout = 5 * time.Second

// Close stops the background flusher and writes what is left, waiting at most CloseTimeout in total. It is safe to
// call more than once.
func (h *Handler) Close() {
	h.c.once.Do(func() { close(h.c.stop) })
	t := time.NewTimer(CloseTimeout)
	defer t.Stop()
	select {
	case <-h.c.done:
	case <-t.C:
		h.c.report(context.Background(), slog.LevelWarn, "run journal: closing timed out, some events may be lost")
	}
}

// Flush flushes the journal behind l, if l has one. Call it when a run (or a step that ends a run's story) is over.
func Flush(l *slog.Logger) {
	if h, ok := l.Handler().(*Handler); ok {
		_ = h.Flush(context.Background())
	}
}

// Enabled reports whether a record of level would be forwarded or stored.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.c.opts.Level || h.inner.Enabled(ctx, level)
}

// Handle forwards r to the process handler and journals it when it belongs to a run.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.inner.Enabled(ctx, r.Level) {
		err = h.inner.Handle(ctx, r)
	}
	if r.Level < h.c.opts.Level {
		return err
	}
	runID, step := h.runID, h.step
	if runID == 0 {
		runID = RunID(ctx)
	}
	attrs := make(map[string]any, len(h.pre)+r.NumAttrs())
	for _, a := range h.pre {
		put(attrs, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		switch {
		case a.Key == AttrRunID && h.prefix == "" && a.Value.Kind() != slog.KindGroup:
			if id, ok := asInt(a.Value); ok {
				runID = id
				return true
			}
		case a.Key == AttrStep && h.prefix == "" && a.Value.Kind() == slog.KindString:
			step = a.Value.String()
			return true
		}
		put(attrs, h.prefix, a)
		return true
	})
	if runID <= 0 {
		return err
	}
	if step == "" {
		step = "run"
	}
	h.c.add(store.RunEvent{RunID: runID, TS: r.Time, Level: levelName(r.Level), Step: clip(step), Message: clip(r.Message), Attrs: encodeAttrs(attrs)})
	return err
}

// WithAttrs returns a handler with attrs added; run_id and step among them set the run and the step of its records.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	n := *h
	n.inner = h.inner.WithAttrs(attrs)
	n.pre = append([]slog.Attr(nil), h.pre...)
	for _, a := range attrs {
		switch {
		case a.Key == AttrRunID && h.prefix == "" && a.Value.Kind() != slog.KindGroup:
			if id, ok := asInt(a.Value); ok {
				n.runID = id
				continue
			}
		case a.Key == AttrStep && h.prefix == "" && a.Value.Kind() == slog.KindString:
			n.step = a.Value.String()
			continue
		}
		a.Key = h.prefix + a.Key
		n.pre = append(n.pre, a)
	}
	return &n
}

// WithGroup returns a handler that nests later attributes under name (dotted keys in the stored attrs).
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.inner = h.inner.WithGroup(name)
	n.prefix = h.prefix + name + "."
	return &n
}

func asInt(v slog.Value) (int64, bool) {
	switch v.Kind() {
	case slog.KindInt64:
		return v.Int64(), true
	case slog.KindUint64:
		if u := v.Uint64(); u <= math.MaxInt64 {
			return int64(u), true
		}
	}
	return 0, false
}

func levelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	}
	return "error"
}

// clip makes s storable: NUL bytes removed (Postgres rejects them in text and jsonb), invalid UTF-8 removed, length
// capped.
func clip(s string) string {
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if len(s) > maxStringLen {
		return strings.ToValidUTF8(s[:maxStringLen], "") + "..."
	}
	return strings.ToValidUTF8(s, "")
}

// encodeAttrs marshals attrs key by key: a value that cannot be encoded is stored as its text and, failing that,
// dropped, so one bad attribute never wipes the others. NUL characters are removed from the result.
func encodeAttrs(attrs map[string]any) json.RawMessage {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	b = append(b, '{')
	first := true
	for _, k := range keys {
		kb, err := json.Marshal(clip(k))
		if err != nil {
			continue
		}
		vb, err := json.Marshal(attrs[k])
		if err != nil {
			vb, err = json.Marshal(clip(fmt.Sprint(attrs[k])))
			if err != nil {
				continue
			}
		}
		if bytes.Contains(vb, nulEscape) {
			vb = stripNULJSON(vb)
		}
		if !first {
			b = append(b, ',')
		}
		first = false
		b = append(b, kb...)
		b = append(b, ':')
		b = append(b, vb...)
	}
	b = append(b, '}')
	if !json.Valid(b) {
		return json.RawMessage("{}")
	}
	return b
}

var nulEscape = []byte(`\u0000`)

// stripNULJSON removes NUL characters from every string (and key) of the JSON value v by decoding and re-encoding it.
func stripNULJSON(v []byte) []byte {
	var x any
	if err := json.Unmarshal(v, &x); err != nil {
		return []byte(`""`)
	}
	out, err := json.Marshal(stripNUL(x))
	if err != nil {
		return []byte(`""`)
	}
	return out
}

func stripNUL(x any) any {
	switch t := x.(type) {
	case string:
		return strings.ReplaceAll(t, "\x00", "")
	case []any:
		for i := range t {
			t[i] = stripNUL(t[i])
		}
		return t
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, e := range t {
			m[strings.ReplaceAll(k, "\x00", "")] = stripNUL(e)
		}
		return m
	}
	return x
}

// sensitive reports whether an attribute name looks like it holds a secret.
func sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range []string{"key", "token", "password", "authorization"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// put stores attribute a under prefix in m: strings, numbers and bools as they are, durations as <key>_ms, groups
// nested, errors as text, anything else as JSON. A text value whose key looks sensitive is replaced by "[redacted]";
// numbers, bools and durations are not secrets and are kept (counters such as "tokens" are common). run_key is not
// stored: it is a column of the run.
func put(m map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if a.Equal(slog.Attr{}) || a.Key == "run_key" && prefix == "" {
		return
	}
	key := prefix + a.Key
	switch v.Kind() {
	case slog.KindGroup:
		for _, g := range v.Group() {
			put(m, key+".", g)
		}
	case slog.KindInt64:
		m[key] = v.Int64()
	case slog.KindUint64:
		m[key] = v.Uint64()
	case slog.KindFloat64:
		if f := v.Float64(); math.IsNaN(f) || math.IsInf(f, 0) {
			m[key] = strconv.FormatFloat(f, 'g', -1, 64) // JSON has no NaN/Inf
		} else {
			m[key] = f
		}
	case slog.KindBool:
		m[key] = v.Bool()
	case slog.KindDuration:
		m[strings.TrimSuffix(key, "_ms")+"_ms"] = v.Duration().Milliseconds()
	case slog.KindTime:
		m[key] = v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindString:
		m[key] = redact(key, clip(v.String()))
	default: // KindAny
		switch x := v.Any().(type) {
		case error:
			m[key] = redact(key, clip(x.Error()))
		case fmt.Stringer:
			m[key] = redact(key, clip(x.String()))
		case []string, []int, []int64, []float64, []bool, map[string]string, map[string]any, map[string]int:
			if sensitive(key) {
				m[key] = "[redacted]"
			} else {
				m[key] = x
			}
		default:
			m[key] = redact(key, clip(fmt.Sprint(x)))
		}
	}
}

func redact(key, val string) string {
	if sensitive(key) {
		return "[redacted]"
	}
	return val
}

// add buffers one event and wakes the flusher when a batch is complete. When a run has more than MaxPerRun buffered
// events the oldest of that run are dropped; one warning event reports the count at the next flush.
func (c *core) add(e store.RunEvent) {
	c.mu.Lock()
	c.buf = append(c.buf, e)
	c.perRun[e.RunID]++
	c.capLocked(e.RunID)
	n := len(c.buf)
	c.mu.Unlock()
	if n >= c.opts.BatchSize {
		select {
		case c.kick <- struct{}{}:
		default:
		}
	}
}

func (c *core) capLocked(runID int64) {
	for c.perRun[runID] > c.opts.MaxPerRun {
		for i := range c.buf {
			if c.buf[i].RunID == runID {
				c.buf = append(c.buf[:i], c.buf[i+1:]...)
				break
			}
		}
		c.perRun[runID]--
		c.dropped[runID]++
	}
}

func (c *core) loop() {
	defer close(c.done)
	t := time.NewTicker(c.opts.Interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-c.kick:
		case <-c.stop:
			_ = c.flush(context.Background())
			return
		}
		_ = c.flush(context.Background())
	}
}

// flush writes the buffer. On a write error the events are put back (subject to the per-run cap) and the error is
// reported once to the process handler.
func (c *core) flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	sp := c.sink.Load()
	if sp == nil {
		return nil
	}
	c.mu.Lock()
	rows := c.buf
	c.buf = nil
	c.perRun = map[int64]int{}
	for id, n := range c.dropped {
		attrs, _ := json.Marshal(map[string]any{"dropped": n})
		rows = append(rows, store.RunEvent{RunID: id, TS: time.Now().UTC(), Level: "warn", Step: "run",
			Message: "run journal buffer overflowed, oldest events were dropped", Attrs: attrs})
	}
	c.dropped = map[int64]int{}
	c.mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
	defer cancel()
	err := (*sp).InsertRunEvents(wctx, rows)
	if err == nil {
		c.fails = 0
		return nil
	}
	if isDataError(err) {
		// A row the database rejects for its content would block the journal forever: store the rest row by row.
		return c.insertEach(wctx, ctx, *sp, rows)
	}
	c.fails++
	if c.fails >= c.opts.MaxAttempts {
		c.fails = 0
		c.drop(ctx, rows, err)
		return fmt.Errorf("runlog: %w", err)
	}
	c.requeue(rows)
	c.report(ctx, slog.LevelError, "run journal: writing events failed, will retry", slog.Int("events", len(rows)),
		slog.Int("attempt", c.fails), slog.Any("err", err))
	return fmt.Errorf("runlog: %w", err)
}

// isDataError reports whether err is a Postgres data exception (class 22) or integrity violation (class 23): the
// statement fails because of the rows, not the connection, so retrying the same rows cannot help.
func isDataError(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && len(pg.Code) >= 2 && (pg.Code[:2] == "22" || pg.Code[:2] == "23")
}

// insertEach writes rows one by one after a batch failed with batchErr. Rows rejected for their content are dropped
// and reported once on the process handler; any other error puts the remaining rows back.
func (c *core) insertEach(wctx, ctx context.Context, sink Sink, rows []store.RunEvent) error {
	for i, row := range rows {
		err := sink.InsertRunEvents(wctx, []store.RunEvent{row})
		switch {
		case err == nil:
		case isDataError(err):
			c.report(ctx, slog.LevelError, "run journal: dropped an event the database rejects",
				slog.Int64("run_id", row.RunID), slog.String("message", head(row.Message, 80)), slog.Any("err", err))
		default:
			c.fails++
			c.requeue(rows[i:])
			c.report(ctx, slog.LevelError, "run journal: writing events failed, will retry", slog.Int("events", len(rows)-i),
				slog.Int("attempt", c.fails), slog.Any("err", err))
			return fmt.Errorf("runlog: %w", err)
		}
	}
	c.fails = 0
	return nil
}

// drop discards a batch that could not be written after MaxAttempts tries: one error on the process handler and one
// warn event per affected run.
func (c *core) drop(ctx context.Context, rows []store.RunEvent, err error) {
	runs := map[int64]int{}
	for _, e := range rows {
		runs[e.RunID]++
	}
	c.report(ctx, slog.LevelError, "run journal: giving up on a batch of events after repeated write errors",
		slog.Int("events", len(rows)), slog.Int("runs", len(runs)), slog.Any("err", err))
	warn := make([]store.RunEvent, 0, len(runs))
	for id, n := range runs {
		attrs, _ := json.Marshal(map[string]any{"dropped": n})
		warn = append(warn, store.RunEvent{RunID: id, TS: time.Now().UTC(), Level: "warn", Step: "run",
			Message: "run journal: events were dropped after repeated write errors", Attrs: attrs})
	}
	sort.Slice(warn, func(i, j int) bool { return warn[i].RunID < warn[j].RunID })
	c.requeue(warn)
}

func (c *core) report(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if c.inner == nil {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(attrs...)
	_ = c.inner.Handle(ctx, r)
}

func head(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "") + "..."
	}
	return s
}

func (c *core) requeue(rows []store.RunEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(rows, c.buf...)
	c.perRun = map[int64]int{}
	for _, e := range c.buf {
		c.perRun[e.RunID]++
	}
	for id := range c.perRun {
		c.capLocked(id)
	}
}
