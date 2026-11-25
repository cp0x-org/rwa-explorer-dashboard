package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

func TestMigrateUpStatusDownUp(t *testing.T) {
	ctx := context.Background()
	u := testdb.Empty(t)

	st, err := db.Status(ctx, u)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(st), 1)
	assert.EqualValues(t, 1, st[0].Version)
	for _, m := range st {
		assert.False(t, m.Applied)
	}

	require.NoError(t, db.Up(ctx, u))
	require.NoError(t, db.Up(ctx, u), "Up is idempotent")
	st, err = db.Status(ctx, u)
	require.NoError(t, err)
	for _, m := range st {
		assert.True(t, m.Applied, m.Source)
	}
	v, err := db.Version(ctx, u)
	require.NoError(t, err)
	assert.EqualValues(t, st[len(st)-1].Version, v)

	pool, err := db.Connect(ctx, u)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('runs','build_exports','raw_snapshots','history_positions')`).Scan(&n))
	assert.Equal(t, 4, n)

	require.NoError(t, db.DownTo(ctx, u, 0))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name NOT LIKE 'goose%'`).Scan(&n))
	assert.Equal(t, 0, n, "down removes every table")
	v, err = db.Version(ctx, u)
	require.NoError(t, err)
	assert.EqualValues(t, 0, v)

	require.NoError(t, db.Up(ctx, u))
	v, err = db.Version(ctx, u)
	require.NoError(t, err)
	assert.EqualValues(t, st[len(st)-1].Version, v)
}

func TestWithTx(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	insert := func(tx pgx.Tx, key string) error {
		_, err := tx.Exec(ctx, `INSERT INTO runs (run_key, mode, trigger, status, started_at, collector_version, settings)
			VALUES ($1, 'refresh', 'cli', 'running', now(), 'test', '{}')`, key)
		return err
	}
	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM runs`).Scan(&n))
		return n
	}

	require.NoError(t, db.WithTx(ctx, pool, func(tx pgx.Tx) error { return insert(tx, "a") }))
	assert.Equal(t, 1, count())

	boom := errors.New("boom")
	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		require.NoError(t, insert(tx, "b"))
		return boom
	})
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 1, count(), "rolled back on error")

	assert.PanicsWithValue(t, "kaboom", func() {
		_ = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
			require.NoError(t, insert(tx, "c"))
			panic("kaboom")
		})
	})
	assert.Equal(t, 1, count(), "rolled back on panic")
}

func TestPoolURLParamsAreNotSentToServer(t *testing.T) {
	ctx := context.Background()
	base := testdb.Empty(t)
	u := base + "&pool_max_conns=5&pool_min_conns=0"
	require.NoError(t, db.Up(ctx, u), "migrate must not forward pool_* params to the server")
	st, err := db.Status(ctx, u)
	require.NoError(t, err)
	assert.True(t, st[0].Applied)

	pool, err := db.Connect(ctx, u)
	require.NoError(t, err)
	defer pool.Close()
	assert.EqualValues(t, 5, pool.Config().MaxConns, "URL overrides the default")

	pool2, err := db.Connect(ctx, base)
	require.NoError(t, err)
	defer pool2.Close()
	assert.EqualValues(t, 8, pool2.Config().MaxConns)
	assert.EqualValues(t, 1, pool2.Config().MinConns)
	assert.Equal(t, 10*time.Second, pool2.Config().ConnConfig.ConnectTimeout)

	var tt time.Time
	require.NoError(t, pool2.QueryRow(ctx, `SELECT now()`).Scan(&tt))
	assert.Equal(t, time.UTC, tt.Location())
}
