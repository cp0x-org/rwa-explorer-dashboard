package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

func TestDealsNewestRowWins(t *testing.T) {
	e := newEnv(t)
	_, err := e.st.LatestDeals(e.ctx)
	require.ErrorIs(t, err, store.ErrNotFound)

	newest := time.Date(2026, 11, 14, 1, 7, 38, 0, time.UTC)
	_, err = e.st.InsertDeals(e.ctx, newest, []byte(`{"fetched_at":"2026-11-14T01:07:38Z","deals":{"a":{"link":"https://a.example.org"}}}`))
	require.NoError(t, err)
	// An older fetch inserted later does not win; neither does an earlier id with the same time.
	_, err = e.st.InsertDeals(e.ctx, newest.Add(-24*time.Hour), []byte(`{"fetched_at":"2026-11-13T01:07:38Z","deals":{}}`))
	require.NoError(t, err)
	payload, err := e.st.LatestDeals(e.ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fetched_at":"2026-11-14T01:07:38Z","deals":{"a":{"link":"https://a.example.org"}}}`, string(payload))

	_, err = e.st.InsertDeals(e.ctx, newest, []byte(`{"fetched_at":"2026-11-14T01:07:38Z","deals":{}}`))
	require.NoError(t, err)
	payload, err = e.st.LatestDeals(e.ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fetched_at":"2026-11-14T01:07:38Z","deals":{}}`, string(payload), "equal times: the later row")
}

func TestLatestSnapshotTokenCounts(t *testing.T) {
	e := newEnv(t)
	add := func(id string, at time.Time, payload string) {
		runID, err := e.st.CreateRun(e.ctx, store.RunParams{RunKey: id + at.Format(time.RFC3339), Mode: "refresh", Trigger: "cli", StartedAt: at, CollectorVersion: "t"})
		require.NoError(t, err)
		_, err = e.st.InsertSnapshot(e.ctx, runID, id, at, "ok", []byte(payload))
		require.NoError(t, err)
	}
	// Latest snapshot decides; tokens without a truthy crypto_id do not count.
	add(nvdaID, day(1, 0), `{"cmc":{"rwa":{"tokens":[{"crypto_id":1},{"crypto_id":2},{"crypto_id":3}]}}}`)
	add(nvdaID, day(2, 0), `{"cmc":{"rwa":{"tokens":[{"crypto_id":1},{"crypto_id":0},{"crypto_id":null},{"symbol":"x"},"junk"]}}}`)
	add(aaplID, day(1, 0), `{"cmc":null,"portals":null}`)

	counts, err := e.st.LatestSnapshotTokenCounts(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{nvdaID: 1, aaplID: 0}, counts)
}
