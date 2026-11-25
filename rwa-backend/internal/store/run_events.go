package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Run event levels in increasing severity.
var eventLevels = []string{"debug", "info", "warn", "error"}

// ValidEventLevel reports whether level is one of debug, info, warn, error.
func ValidEventLevel(level string) bool { return levelRank(level) >= 0 }

func levelRank(level string) int {
	for i, l := range eventLevels {
		if l == level {
			return i
		}
	}
	return -1
}

// RunEvent is a run_events row. ID is 0 before the insert.
type RunEvent struct {
	ID      int64
	RunID   int64
	TS      time.Time
	Level   string // debug | info | warn | error
	Step    string
	Message string
	Attrs   json.RawMessage // JSON object
}

// InsertRunEvents stores rows in order in one statement. Rows of runs that no longer exist are skipped, so a deleted
// run cannot fail a batch.
func (s *Store) InsertRunEvents(ctx context.Context, rows []RunEvent) error {
	if len(rows) == 0 {
		return nil
	}
	runIDs := make([]int64, len(rows))
	ts := make([]time.Time, len(rows))
	levels := make([]string, len(rows))
	steps := make([]string, len(rows))
	msgs := make([]string, len(rows))
	attrs := make([]string, len(rows))
	for i, r := range rows {
		runIDs[i], ts[i], levels[i], steps[i], msgs[i] = r.RunID, r.TS, r.Level, r.Step, r.Message
		attrs[i] = string(jsonOr(r.Attrs, "{}"))
	}
	_, err := s.q.Exec(ctx, `INSERT INTO run_events (run_id, ts, level, step, message, attrs)
		SELECT e.run_id, e.ts, e.level, e.step, e.message, e.attrs::jsonb
		FROM unnest($1::bigint[], $2::timestamptz[], $3::text[], $4::text[], $5::text[], $6::text[])
			WITH ORDINALITY AS e(run_id, ts, level, step, message, attrs, n)
		WHERE EXISTS (SELECT 1 FROM runs r WHERE r.id = e.run_id)
		ORDER BY e.n`, runIDs, ts, levels, steps, msgs, attrs)
	if err != nil {
		return fmt.Errorf("store: insert run events: %w", err)
	}
	return nil
}

// ListRunEvents returns the events of a run with id > afterID and level >= level (debug < info < warn < error; empty
// means debug), oldest first. limit is clamped to 1..2000. nextAfter is the id to continue from when more events
// exist, else nil.
func (s *Store) ListRunEvents(ctx context.Context, runID, afterID int64, level string, limit int) ([]RunEvent, *int64, error) {
	rank := levelRank(level)
	if level == "" {
		rank = 0
	}
	if rank < 0 {
		return nil, nil, fmt.Errorf("store: list run events: invalid level %q", level)
	}
	limit = min(max(limit, 1), 2000)
	rows, err := s.q.Query(ctx, `SELECT id, run_id, ts, level, step, message, attrs FROM run_events
		WHERE run_id = $1 AND id > $2 AND level = ANY($3) ORDER BY id LIMIT $4`, runID, afterID, eventLevels[rank:], limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("store: list run events: %w", err)
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		var e RunEvent
		if err := rows.Scan(&e.ID, &e.RunID, &e.TS, &e.Level, &e.Step, &e.Message, &e.Attrs); err != nil {
			return nil, nil, fmt.Errorf("store: list run events: scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("store: list run events: %w", err)
	}
	var next *int64
	if len(out) > limit {
		out = out[:limit]
		id := out[limit-1].ID
		next = &id
	}
	return out, next, nil
}

// RunEventsKeepNewest is how many of the newest collection runs keep their events whatever the retention cutoff.
const RunEventsKeepNewest = 3

// journalStaleAfter is how long a finished run may stay without any event and without a terminal event before its
// journal counts as closed anyway (runs of older versions, a writer that crashed).
const journalStaleAfter = 15 * time.Minute

// DeleteRunEventsOlderThan deletes the events of the runs that started before cutoff, whole runs only (a run's story is
// never split). The events of the RunEventsKeepNewest newest runs and of the run behind the current build are kept.
func (s *Store) DeleteRunEventsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `DELETE FROM run_events WHERE run_id IN (
			SELECT id FROM runs WHERE started_at < $1
				AND id NOT IN (SELECT id FROM runs ORDER BY started_at DESC, id DESC LIMIT $2)
				AND id NOT IN (SELECT run_id FROM build_exports WHERE is_current))`, cutoff, RunEventsKeepNewest)
	if err != nil {
		return 0, fmt.Errorf("store: delete old run events: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunStatus returns the status of a run (ErrNotFound when it does not exist): one cheap query, unlike GetRun.
func (s *Store) RunStatus(ctx context.Context, id int64) (string, error) {
	var status string
	if err := s.q.QueryRow(ctx, `SELECT status FROM runs WHERE id = $1`, id).Scan(&status); err != nil {
		return "", fmt.Errorf("store: run %d status: %w", id, notFound(err))
	}
	return status, nil
}

// RunJournalClosed reports whether the job that owns the run has finished writing: a terminal event (step run,
// message "journal closed") exists, or the run has had no event for a long time.
func (s *Store) RunJournalClosed(ctx context.Context, id int64) (bool, error) {
	var closed bool
	err := s.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM run_events WHERE run_id = $1 AND step = 'run' AND message = 'journal closed')
		OR NOT EXISTS (SELECT 1 FROM run_events WHERE run_id = $1 AND ts > now() - $2::interval)`, id, journalStaleAfter).Scan(&closed)
	if err != nil {
		return false, fmt.Errorf("store: run %d journal state: %w", id, err)
	}
	return closed, nil
}

// RunEventCounts returns the number of events of a run and how many of them have level error.
func (s *Store) RunEventCounts(ctx context.Context, runID int64) (total, errors int, err error) {
	err = s.q.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE level = 'error') FROM run_events WHERE run_id = $1`, runID).Scan(&total, &errors)
	if err != nil {
		return 0, 0, fmt.Errorf("store: count run events: %w", err)
	}
	return total, errors, nil
}
