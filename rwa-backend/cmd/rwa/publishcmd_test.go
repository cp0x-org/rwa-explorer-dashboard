package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

func TestCheckPublishable(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	st := store.New(pool)
	mk := func(key, mode, status, errText string) *store.Run {
		t.Helper()
		id, err := st.CreateRun(ctx, store.RunParams{RunKey: key, Mode: mode, Trigger: "cli", StartedAt: time.Now(), CollectorVersion: "t"})
		require.NoError(t, err)
		if status != "running" {
			require.NoError(t, st.FinishRun(ctx, id, status, store.RunCounters{}, nil, errText))
		}
		d, err := st.GetRun(ctx, id)
		require.NoError(t, err)
		return &d.Run
	}

	require.NoError(t, checkPublishable(ctx, st, mk("ok", "refresh", "succeeded", "")))
	require.NoError(t, checkPublishable(ctx, st, mk("partial", "add", "partial", "")))
	require.NoError(t, checkPublishable(ctx, st, mk("pubfail", "refresh", "failed", "publish: boom")), "a failed publish can be retried")
	require.ErrorContains(t, checkPublishable(ctx, st, mk("interrupted", "refresh", "failed", "interrupted")), "failed during collection")
	require.ErrorContains(t, checkPublishable(ctx, st, mk("running", "refresh", "running", "")), "still running")
	require.ErrorContains(t, checkPublishable(ctx, st, mk("rb", "rebuild", "succeeded", "")), "rebuild")

	built := mk("built", "refresh", "succeeded", "")
	require.NoError(t, db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := st.PublishBuild(ctx, tx, store.BuildRow{RunID: built.ID, GeneratedAt: time.Now(), SchemaVersion: 1, ScoreVersion: "v1", Export: []byte(`{}`), ExportSHA256: "x"}, nil)
		return err
	}))
	require.ErrorContains(t, checkPublishable(ctx, st, built), "already has a build")
}
