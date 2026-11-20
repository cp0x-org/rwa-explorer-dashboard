package store_test

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

func TestBuildChangesForRuns(t *testing.T) {
	e := newEnv(t)
	ctx, st := e.ctx, e.st
	noBuild := e.run(t, "r-nobuild", day(1, 6))
	noChanges := e.run(t, "r-nochanges", day(2, 6))
	multi := e.run(t, "r-multi", day(3, 6))

	build := func(runID int64, sha string, ch *store.BuildChangesRow) int64 {
		var id int64
		require.NoError(t, db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
			var err error
			id, err = st.PublishBuild(ctx, tx, store.BuildRow{RunID: runID, GeneratedAt: day(4, 6), SchemaVersion: 1, ScoreVersion: "v1",
				Export: []byte(`{}`), ExportSHA256: sha}, ch)
			return err
		}))
		return id
	}
	nc := build(noChanges, "a", nil)
	build(multi, "b", &store.BuildChangesRow{NewTokens: []int32{1}, EligibleDelta: 1})
	last := build(multi, "c", &store.BuildChangesRow{NewTokens: []int32{1, 2}, NewIssuers: []string{"x"}, EligibleDelta: 5, Details: []byte(`{"k":1}`)})

	got, err := st.BuildChangesForRuns(ctx, []int64{noBuild, noChanges, multi})
	require.NoError(t, err)
	assert.NotContains(t, got, noBuild, "a run without a build is absent")
	require.Contains(t, got, noChanges)
	assert.Equal(t, nc, got[noChanges].BuildID)
	assert.False(t, got[noChanges].HasChanges)
	assert.Equal(t, 0, got[noChanges].EligibleDelta)
	require.Contains(t, got, multi)
	assert.Equal(t, last, got[multi].BuildID, "the newest build of the run wins")
	assert.True(t, got[multi].HasChanges)
	assert.Equal(t, []int32{1, 2}, got[multi].NewTokens)
	assert.Equal(t, 5, got[multi].EligibleDelta)

	empty, err := st.BuildChangesForRuns(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestGetRunRequest(t *testing.T) {
	e := newEnv(t)
	_, err := e.st.GetRunRequest(e.ctx, 12345)
	require.ErrorIs(t, err, store.ErrNotFound)
	id, err := e.st.CreateRunRequest(e.ctx, "admin-api", "add", []string{"NVDA"})
	require.NoError(t, err)
	r, err := e.st.GetRunRequest(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "add", r.Mode)
	assert.Equal(t, []string{"NVDA"}, r.Symbols)
	assert.Nil(t, r.Outcome)
}
