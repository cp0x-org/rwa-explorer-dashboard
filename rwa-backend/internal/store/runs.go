package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RunParams starts a collection run.
type RunParams struct {
	RunKey           string // 20261018T201453Z
	Mode             string // refresh | add | import | rebuild
	Trigger          string // schedule | startup | manual | import | cli
	StartedAt        time.Time
	CollectorVersion string
	Settings         []byte // settings.json the run uses (JSON)
}

// RunCounters are the per-run totals written at the end of a run.
type RunCounters struct {
	Selected        int
	Completed       int
	Failed          int
	CMCRequests     int
	PortalsRequests int
	CMCCredits      int
}

// Run is a collection_runs row.
type Run struct {
	ID               int64
	RunKey           string
	Mode             string
	Trigger          string
	Status           string // running | succeeded | partial | failed
	StartedAt        time.Time
	FinishedAt       *time.Time
	Counters         RunCounters
	Warnings         json.RawMessage
	Error            *string
	CollectorVersion string
	Settings         json.RawMessage
}

// RunUnderlying is a run_underlyings row.
type RunUnderlying struct {
	RunID           int64
	UnderlyingID    string
	Status          string
	Error           *string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	PortalsRequests int
}

// RunDetail is a run with its per-underlying results.
type RunDetail struct {
	Run
	Underlyings []RunUnderlying
}

// RunCursor is the keyset position for ListRuns pagination: the last run of the previous page.
type RunCursor struct {
	StartedAt time.Time
	ID        int64
}

// RunRequest is a manual run request written by the API and consumed by the collector.
type RunRequest struct {
	ID          int64
	RequestedAt time.Time
	RequestedBy string
	Mode        string
	Symbols     []string // nil = all enabled
	TakenAt     *time.Time
	RunID       *int64
}

const runColumns = `id, run_key, mode, trigger, status, started_at, finished_at, selected_count, completed_count,
	failed_count, cmc_requests, portals_requests, cmc_credits, warnings, error, collector_version, settings`

func scanRun(row pgx.Row) (*Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.RunKey, &r.Mode, &r.Trigger, &r.Status, &r.StartedAt, &r.FinishedAt,
		&r.Counters.Selected, &r.Counters.Completed, &r.Counters.Failed, &r.Counters.CMCRequests,
		&r.Counters.PortalsRequests, &r.Counters.CMCCredits, &r.Warnings, &r.Error, &r.CollectorVersion, &r.Settings)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateRun inserts a run with status 'running' and returns its id.
func (s *Store) CreateRun(ctx context.Context, p RunParams) (int64, error) {
	var id int64
	err := s.q.QueryRow(ctx, `INSERT INTO collection_runs (run_key, mode, trigger, status, started_at, collector_version, settings)
		VALUES ($1, $2, $3, 'running', $4, $5, $6) RETURNING id`,
		p.RunKey, p.Mode, p.Trigger, p.StartedAt, p.CollectorVersion, jsonOr(p.Settings, "{}")).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: insert run: %w", err)
	}
	return id, nil
}

// FinishRun records the final status, counters, warnings and fatal error (empty = none).
func (s *Store) FinishRun(ctx context.Context, id int64, status string, c RunCounters, warnings []string, errText string) error {
	if warnings == nil {
		warnings = []string{}
	}
	wb, err := json.Marshal(warnings)
	if err != nil {
		return fmt.Errorf("store: finish run: marshal warnings: %w", err)
	}
	var errArg *string
	if errText != "" {
		errArg = &errText
	}
	tag, err := s.q.Exec(ctx, `UPDATE collection_runs SET status = $2, finished_at = now(), selected_count = $3,
		completed_count = $4, failed_count = $5, cmc_requests = $6, portals_requests = $7, cmc_credits = $8,
		warnings = $9, error = $10 WHERE id = $1`,
		id, status, c.Selected, c.Completed, c.Failed, c.CMCRequests, c.PortalsRequests, c.CMCCredits, wb, errArg)
	if err != nil {
		return fmt.Errorf("store: finish run %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("store: finish run %d: %w", id, ErrNotFound)
	}
	return nil
}

// UpsertRunUnderlying inserts or updates the per-underlying result of a run.
func (s *Store) UpsertRunUnderlying(ctx context.Context, u RunUnderlying) error {
	_, err := s.q.Exec(ctx, `INSERT INTO run_underlyings (run_id, underlying_id, status, error, started_at, finished_at, portals_requests)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (run_id, underlying_id) DO UPDATE SET status = EXCLUDED.status, error = EXCLUDED.error,
			started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at, portals_requests = EXCLUDED.portals_requests`,
		u.RunID, u.UnderlyingID, u.Status, u.Error, u.StartedAt, u.FinishedAt, u.PortalsRequests)
	if err != nil {
		return fmt.Errorf("store: upsert run underlying %s: %w", u.UnderlyingID, err)
	}
	return nil
}

// LastRun returns the run with the latest started_at, or ErrNotFound.
func (s *Store) LastRun(ctx context.Context) (*Run, error) {
	r, err := scanRun(s.q.QueryRow(ctx, `SELECT `+runColumns+` FROM collection_runs ORDER BY started_at DESC, id DESC LIMIT 1`))
	if err != nil {
		return nil, fmt.Errorf("store: last run: %w", notFound(err))
	}
	return r, nil
}

// ListRuns returns runs newest first (limit clamped to 1..500; an empty page is an empty slice). Pass the last
// row of the previous page as cursor; nil starts from the newest. Timestamps are stored with microsecond
// precision, so the cursor is truncated to microseconds before comparison.
func (s *Store) ListRuns(ctx context.Context, limit int, cursor *RunCursor) ([]Run, error) {
	limit = min(max(limit, 1), 500)
	var rows pgx.Rows
	var err error
	if cursor == nil {
		rows, err = s.q.Query(ctx, `SELECT `+runColumns+` FROM collection_runs ORDER BY started_at DESC, id DESC LIMIT $1`, limit)
	} else {
		rows, err = s.q.Query(ctx, `SELECT `+runColumns+` FROM collection_runs WHERE (started_at, id) < ($2, $3)
			ORDER BY started_at DESC, id DESC LIMIT $1`, limit, cursor.StartedAt.Truncate(time.Microsecond), cursor.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list runs: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list runs: scan: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list runs: %w", err)
	}
	return out, nil
}

// GetRun returns a run with its underlyings, or ErrNotFound.
func (s *Store) GetRun(ctx context.Context, id int64) (*RunDetail, error) {
	r, err := scanRun(s.q.QueryRow(ctx, `SELECT `+runColumns+` FROM collection_runs WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("store: get run %d: %w", id, notFound(err))
	}
	rows, err := s.q.Query(ctx, `SELECT run_id, underlying_id, status, error, started_at, finished_at, portals_requests
		FROM run_underlyings WHERE run_id = $1 ORDER BY underlying_id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: get run %d: underlyings: %w", id, err)
	}
	defer rows.Close()
	d := &RunDetail{Run: *r}
	for rows.Next() {
		var u RunUnderlying
		if err := rows.Scan(&u.RunID, &u.UnderlyingID, &u.Status, &u.Error, &u.StartedAt, &u.FinishedAt, &u.PortalsRequests); err != nil {
			return nil, fmt.Errorf("store: get run %d: scan underlying: %w", id, err)
		}
		d.Underlyings = append(d.Underlyings, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get run %d: underlyings: %w", id, err)
	}
	return d, nil
}

// CreateRunRequest records a manual run request (symbols nil = all enabled) and returns its id.
func (s *Store) CreateRunRequest(ctx context.Context, requestedBy, mode string, symbols []string) (int64, error) {
	if len(symbols) == 0 {
		symbols = nil // NULL = all enabled
	}
	var id int64
	err := s.q.QueryRow(ctx, `INSERT INTO run_requests (requested_by, mode, symbols) VALUES ($1, $2, $3) RETURNING id`,
		requestedBy, mode, symbols).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: insert run request: %w", err)
	}
	return id, nil
}

// TakeNextRunRequest claims the oldest pending request (FOR UPDATE SKIP LOCKED, one atomic statement)
// and sets taken_at. It returns ErrNotFound when nothing is pending.
func (s *Store) TakeNextRunRequest(ctx context.Context) (*RunRequest, error) {
	var r RunRequest
	err := s.q.QueryRow(ctx, `UPDATE run_requests SET taken_at = now()
		WHERE id = (SELECT id FROM run_requests WHERE taken_at IS NULL ORDER BY requested_at, id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, requested_at, requested_by, mode, symbols, taken_at, run_id`).
		Scan(&r.ID, &r.RequestedAt, &r.RequestedBy, &r.Mode, &r.Symbols, &r.TakenAt, &r.RunID)
	if err != nil {
		return nil, fmt.Errorf("store: take run request: %w", notFound(err))
	}
	return &r, nil
}

// AttachRunToRequest links the run started for a request.
func (s *Store) AttachRunToRequest(ctx context.Context, requestID, runID int64) error {
	tag, err := s.q.Exec(ctx, `UPDATE run_requests SET run_id = $2 WHERE id = $1`, requestID, runID)
	if err != nil {
		return fmt.Errorf("store: attach run %d to request %d: %w", runID, requestID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("store: attach run to request %d: %w", requestID, ErrNotFound)
	}
	return nil
}
