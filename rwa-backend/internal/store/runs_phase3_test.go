package store_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

func TestRunRequestReleaseAndFinish(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	id, err := st.CreateRunRequest(ctx, "admin-api", "add", []string{"NVDA"})
	require.NoError(t, err)

	got, err := st.TakeNextRunRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	_, err = st.TakeNextRunRequest(ctx)
	require.ErrorIs(t, err, store.ErrNotFound, "a taken request is not offered again")

	// Released: pending again, with the reason visible until it is taken once more.
	require.NoError(t, st.ReleaseRunRequest(ctx, id, "another run in progress"))
	var taken *string
	var outcome, errText *string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT taken_at::text, outcome, error FROM run_requests WHERE id = $1`, id).Scan(&taken, &outcome, &errText))
	assert.Nil(t, taken)
	require.NotNil(t, outcome)
	assert.Equal(t, store.OutcomeReleased, *outcome)
	require.NotNil(t, errText)
	assert.Equal(t, "another run in progress", *errText)

	runID := e.run(t, "r1", day(1, 6))
	require.NoError(t, st.AttachRunToRequest(ctx, id, runID))
	again, err := st.TakeNextRunRequest(ctx)
	require.NoError(t, err)
	assert.Equal(t, id, again.ID)
	assert.Nil(t, again.Outcome, "taking a released request clears the previous outcome")
	assert.Nil(t, again.Error)
	assert.Nil(t, again.RunID, "and the run link of the abandoned attempt")

	require.NoError(t, st.FinishRunRequest(ctx, id, store.OutcomeRejected, errors.New("unknown or disabled symbols: NOPE")))
	var finished bool
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT finished_at IS NOT NULL, outcome, error FROM run_requests WHERE id = $1`, id).Scan(&finished, &outcome, &errText))
	assert.True(t, finished)
	assert.Equal(t, store.OutcomeRejected, *outcome)
	assert.Contains(t, *errText, "NOPE")

	// A finished request cannot be released or served again.
	assert.ErrorIs(t, st.ReleaseRunRequest(ctx, id, ""), store.ErrNotFound)
	_, err = st.TakeNextRunRequest(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Error(t, st.FinishRunRequest(ctx, id, "released", nil), "released is not a final outcome")
	assert.ErrorIs(t, st.FinishRunRequest(ctx, 999, store.OutcomeDone, nil), store.ErrNotFound)
}

func TestMarkRunPublishFailedAndRestore(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	id := e.run(t, "r1", day(1, 6))
	require.NoError(t, st.FinishRun(ctx, id, "succeeded", store.RunCounters{Selected: 2, Completed: 2}, nil, ""))

	require.NoError(t, st.MarkRunPublishFailed(ctx, id, errors.New("boom")))
	run, err := st.GetRun(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	require.NotNil(t, run.Error)
	assert.Equal(t, "publish: boom", *run.Error)
	assert.ErrorIs(t, st.MarkRunPublishFailed(ctx, 999, errors.New("x")), store.ErrNotFound)

	require.NoError(t, st.ClearRunPublishFailed(ctx, id))
	run, err = st.GetRun(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "succeeded", run.Status)
	assert.Nil(t, run.Error)

	// A run that failed for another reason is not resurrected.
	other := e.run(t, "r2", day(2, 6))
	require.NoError(t, st.FinishRun(ctx, other, "failed", store.RunCounters{}, nil, "interrupted"))
	require.NoError(t, st.ClearRunPublishFailed(ctx, other))
	run, err = st.GetRun(ctx, other)
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
}

func TestLastSuccessfulRunStart(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	_, err := st.LastSuccessfulRunStart(ctx)
	require.ErrorIs(t, err, store.ErrNotFound)

	publish := func(runID int64, n int) {
		t.Helper()
		require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
			_, err := st.PublishBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(n, 7), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "x"}, nil)
			return err
		}))
	}
	mk := func(key, mode, status string, d int) int64 {
		t.Helper()
		id, err := st.CreateRun(ctx, store.RunParams{RunKey: key, Mode: mode, Trigger: "cli", StartedAt: day(d, 6), CollectorVersion: "t"})
		require.NoError(t, err)
		require.NoError(t, st.FinishRun(ctx, id, status, store.RunCounters{}, nil, ""))
		return id
	}

	// Finished but never published (no build): not fresh.
	mk("nobuild", "refresh", "succeeded", 1)
	_, err = st.LastSuccessfulRunStart(ctx)
	require.ErrorIs(t, err, store.ErrNotFound)

	r2 := mk("old", "refresh", "partial", 2)
	publish(r2, 2)
	// Newer but unsuitable: an add run with a build, a rebuild, a failed run with a build.
	publish(mk("add", "add", "succeeded", 3), 3)
	publish(mk("rebuild", "rebuild", "succeeded", 4), 4)
	publish(mk("failed", "refresh", "failed", 5), 5)
	mk("nobuild2", "refresh", "succeeded", 6)

	got, err := st.LastSuccessfulRunStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, day(2, 6), got)

	publish(mk("imp", "import", "succeeded", 7), 7)
	got, err = st.LastSuccessfulRunStart(ctx)
	require.NoError(t, err)
	assert.Equal(t, day(7, 6), got)
}

func TestAbandonRunningRuns(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	running := e.run(t, "r1", day(1, 6))
	done := e.run(t, "r2", day(2, 6))
	require.NoError(t, st.FinishRun(ctx, done, "succeeded", store.RunCounters{}, nil, ""))

	n, err := st.AbandonRunningRuns(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	run, err := st.GetRun(ctx, running)
	require.NoError(t, err)
	assert.Equal(t, "failed", run.Status)
	require.NotNil(t, run.Error)
	assert.Equal(t, "abandoned", *run.Error)
	assert.NotNil(t, run.FinishedAt)
	n, err = st.AbandonRunningRuns(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestReserveBuildIDAndPresetInsert(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	runID := e.run(t, "r1", day(1, 6))
	id, err := st.ReserveBuildID(ctx)
	require.NoError(t, err)
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		got, err := st.InsertBuild(ctx, tx, store.BuildRow{ID: id, RunID: runID, GeneratedAt: day(1, 7), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "a"})
		assert.Equal(t, id, got)
		return err
	}))
	// Without a preset id the sequence continues after the reserved one.
	var next int64
	require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) (err error) {
		next, err = st.InsertBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(1, 8), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "b"})
		return err
	}))
	assert.Greater(t, next, id)
	has, err := st.RunHasBuild(ctx, runID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestUpsertUnderlyingFactsKeepInfo(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	runID := e.run(t, "r1", day(1, 6))
	site, industry := "https://nvidia.com", "Tech"
	require.NoError(t, st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{{ID: nvdaID, Website: &site, Industry: &industry, LastRunID: runID}}))
	require.NoError(t, st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{{ID: nvdaID, LastRunID: runID, KeepInfo: true}}))
	var got, ind *string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT website, industry FROM catalog_underlyings WHERE id = $1`, nvdaID).Scan(&got, &ind))
	require.NotNil(t, got)
	assert.Equal(t, site, *got)
	require.NotNil(t, ind)
	assert.Equal(t, industry, *ind)
	require.NoError(t, st.UpsertUnderlyingFacts(ctx, []store.UnderlyingFactsRow{{ID: nvdaID, LastRunID: runID}}))
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT website FROM catalog_underlyings WHERE id = $1`, nvdaID).Scan(&got))
	assert.Nil(t, got, "without KeepInfo the facts are overwritten (publish, import)")
}
