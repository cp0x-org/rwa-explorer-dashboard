package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

func ev(runID int64, ts time.Time, level, msg string) store.RunEvent {
	return store.RunEvent{RunID: runID, TS: ts, Level: level, Step: "run", Message: msg, Attrs: json.RawMessage(`{"k":"v"}`)}
}

func TestRunEventsInsertListAndCounts(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	now := time.Now().UTC()
	run := e.run(t, "r1", now)

	// a row of a missing run is skipped instead of failing the batch
	require.NoError(t, st.InsertRunEvents(ctx, []store.RunEvent{
		ev(run, now, "debug", "d"), ev(run, now, "info", "i1"), ev(9999, now, "info", "ghost"),
		ev(run, now, "warn", "w"), ev(run, now, "error", "e"), ev(run, now, "info", "i2")}))
	require.NoError(t, st.InsertRunEvents(ctx, nil))
	require.Error(t, st.InsertRunEvents(ctx, []store.RunEvent{ev(run, now, "trace", "bad level")}))

	all, next, err := st.ListRunEvents(ctx, run, 0, "debug", 100)
	require.NoError(t, err)
	require.Len(t, all, 5)
	assert.Nil(t, next)
	for i := 1; i < len(all); i++ {
		assert.Greater(t, all[i].ID, all[i-1].ID, "insert order kept")
	}
	assert.Equal(t, []string{"d", "i1", "w", "e", "i2"}, []string{all[0].Message, all[1].Message, all[2].Message, all[3].Message, all[4].Message})
	assert.JSONEq(t, `{"k":"v"}`, string(all[0].Attrs))

	info, _, err := st.ListRunEvents(ctx, run, 0, "info", 100)
	require.NoError(t, err)
	assert.Len(t, info, 4)
	warn, _, err := st.ListRunEvents(ctx, run, 0, "warn", 100)
	require.NoError(t, err)
	assert.Len(t, warn, 2)
	errs, _, err := st.ListRunEvents(ctx, run, 0, "error", 100)
	require.NoError(t, err)
	assert.Len(t, errs, 1)
	_, _, err = st.ListRunEvents(ctx, run, 0, "trace", 100)
	require.Error(t, err)

	// cursor
	p1, next, err := st.ListRunEvents(ctx, run, 0, "debug", 2)
	require.NoError(t, err)
	require.Len(t, p1, 2)
	require.NotNil(t, next)
	assert.Equal(t, p1[1].ID, *next)
	p2, next, err := st.ListRunEvents(ctx, run, *next, "debug", 2)
	require.NoError(t, err)
	require.Len(t, p2, 2)
	assert.Equal(t, all[2].ID, p2[0].ID, "strict id > after")
	require.NotNil(t, next)
	p3, next, err := st.ListRunEvents(ctx, run, *next, "debug", 2)
	require.NoError(t, err)
	require.Len(t, p3, 1)
	assert.Nil(t, next)

	empty, next, err := st.ListRunEvents(ctx, 12345, 0, "debug", 10)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
	assert.Nil(t, next)

	total, nerr, err := st.RunEventCounts(ctx, run)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Equal(t, 1, nerr)
	d, err := st.GetRun(ctx, run)
	require.NoError(t, err)
	assert.Equal(t, 5, d.EventsCount)
	assert.Equal(t, 1, d.ErrorsCount)

	// deleting the run deletes its events
	_, err = e.pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, run)
	require.NoError(t, err)
	total, _, err = st.RunEventCounts(ctx, run)
	require.NoError(t, err)
	assert.Zero(t, total)
}

func TestDeleteRunEventsOlderThanKeepsWholeRuns(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	oldRun := e.run(t, "old", old)
	currentRun := e.run(t, "current", old.Add(time.Hour))
	oldest := e.run(t, "n1", now.Add(-3*time.Hour))
	n2 := e.run(t, "n2", now.Add(-2*time.Hour))
	n3 := e.run(t, "n3", now.Add(-time.Hour))
	// an old run still writing: events dated after the cutoff must go together with the old ones, never split
	require.NoError(t, st.InsertRunEvents(ctx, []store.RunEvent{
		ev(oldRun, old, "info", "old"), ev(oldRun, now, "info", "recent of old run"),
		ev(currentRun, old, "info", "old of current"), ev(oldest, now, "info", "a"), ev(n2, now, "info", "b"), ev(n3, now, "info", "c")}))
	_, err := e.pool.Exec(ctx, `INSERT INTO build_exports (run_id, generated_at, schema_version, score_version, settings, summary, export, export_sha256, is_current)
		VALUES ($1, now(), 1, 'v', '{}', '{}', '\x7b7d', 'x', TRUE)`, currentRun)
	require.NoError(t, err)

	n, err := st.DeleteRunEventsOlderThan(ctx, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "the whole old run, none of the current build's run or the 3 newest")
	for run, want := range map[int64]int{oldRun: 0, currentRun: 1, oldest: 1, n2: 1, n3: 1} {
		total, _, err := st.RunEventCounts(ctx, run)
		require.NoError(t, err)
		assert.Equal(t, want, total, "run %d", run)
	}
}

func TestRunStatusAndJournalClosed(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	now := time.Now().UTC()
	run := e.run(t, "r", now)
	status, err := st.RunStatus(ctx, run)
	require.NoError(t, err)
	assert.NotEmpty(t, status)
	_, err = st.RunStatus(ctx, 99999)
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, st.InsertRunEvents(ctx, []store.RunEvent{ev(run, now, "info", "run finished")}))
	closed, err := st.RunJournalClosed(ctx, run)
	require.NoError(t, err)
	assert.False(t, closed, "recent events, no terminal event")
	require.NoError(t, st.InsertRunEvents(ctx, []store.RunEvent{ev(run, now, "info", "journal closed")}))
	closed, err = st.RunJournalClosed(ctx, run)
	require.NoError(t, err)
	assert.True(t, closed)

	stale := e.run(t, "stale", now.Add(-2*time.Hour))
	require.NoError(t, st.InsertRunEvents(ctx, []store.RunEvent{ev(stale, now.Add(-time.Hour), "info", "run finished")}))
	closed, err = st.RunJournalClosed(ctx, stale)
	require.NoError(t, err)
	assert.True(t, closed, "no event for a long time")
}
