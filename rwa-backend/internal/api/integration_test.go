package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

// TestIntegration runs the API over a real database seeded with a build made from the golden export.
func TestIntegration(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	st := store.New(pool)

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "rwa.json"))
	require.NoError(t, err)

	started := time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC)
	runID, err := st.CreateRun(ctx, store.RunParams{RunKey: "20261018T200000Z", Mode: "refresh", Trigger: "schedule",
		StartedAt: started, CollectorVersion: "test-1", Settings: []byte(`{}`)})
	require.NoError(t, err)
	require.NoError(t, st.FinishRun(ctx, runID, "succeeded", store.RunCounters{Selected: 125, Completed: 125}, []string{"careful"}, ""))
	buildID, err := st.ReserveBuildID(ctx)
	require.NoError(t, err)
	// Stamp ids into the bytes the way publishing does (replace the opening brace area).
	stamped := append([]byte(nil), raw...)
	stamped = append([]byte(`{"build_id": `+itoa(buildID)+`, "run_id": `+itoa(runID)+`,`), stamped[1:]...)
	sum := sha256.Sum256(stamped)
	sha := hex.EncodeToString(sum[:])
	require.NoError(t, db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := st.PublishBuild(ctx, tx, store.BuildRow{ID: buildID, RunID: runID, GeneratedAt: started, SchemaVersion: 1,
			ScoreVersion: "v1", Settings: []byte(`{}`), Summary: []byte(`{}`), Export: stamped, ExportSHA256: sha},
			&store.BuildChangesRow{NewTokens: []int32{1}, NewPositionsCount: 5, EligibleDelta: 2, Details: []byte(`{}`)})
		return err
	}))
	_, err = st.InsertLogoIfAbsent(ctx, store.Logo{Kind: "tokens", EntityID: "36992", Ext: "png", ContentType: "image/png",
		Bytes: []byte("png-bytes"), SHA256: "deadbeef", Source: "test"})
	require.NoError(t, err)
	require.NoError(t, st.InsertSummaryHistory(ctx, []store.SummaryHistoryRow{{RunID: runID, ObservedAt: started, TokensCount: 647}}))

	s := New(&config.Config{AdminToken: "tok"}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, s.cache.refresh(ctx))
	h := s.Handler()

	rec := do(h, "GET", "/readyz", nil, "")
	assert.Equal(t, 200, rec.Code)

	rec = do(h, "GET", "/api/v1/export", nil, "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, stamped, rec.Body.Bytes(), "bytes round-trip through BYTEA verbatim")
	assert.Equal(t, `"`+sha+`"`, rec.Header().Get("ETag"))
	assert.Equal(t, 304, do(h, "GET", "/api/v1/export", map[string]string{"If-None-Match": `"` + sha + `"`}, "").Code)
	assert.Equal(t, 200, do(h, "GET", "/api/v1/export?build="+itoa(buildID), nil, "").Code)

	var status map[string]any
	jsonBody(t, do(h, "GET", "/api/v1/status", nil, ""), &status)
	assert.Equal(t, "test-1", status["collector_version"])
	assert.Equal(t, sha, status["current_build"].(map[string]any)["export_sha256"])
	assert.Equal(t, "succeeded", status["last_run"].(map[string]any)["status"])

	var runs struct {
		Items []map[string]any `json:"items"`
	}
	jsonBody(t, do(h, "GET", "/api/v1/runs", nil, ""), &runs)
	require.Len(t, runs.Items, 1)
	assert.EqualValues(t, buildID, runs.Items[0]["build_id"])
	assert.EqualValues(t, 1, runs.Items[0]["changes"].(map[string]any)["new_tokens_count"])
	assert.EqualValues(t, 1, runs.Items[0]["warnings_count"])
	var run map[string]any
	jsonBody(t, do(h, "GET", "/api/v1/runs/"+itoa(runID), nil, ""), &run)
	assert.Equal(t, []any{"careful"}, run["warnings"])
	assert.Equal(t, []any{float64(1)}, run["changes"].(map[string]any)["new_tokens"])

	rec = do(h, "GET", "/logos/tokens/36992.png", nil, "")
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "png-bytes", rec.Body.String())
	assert.Equal(t, 404, do(h, "GET", "/logos/tokens/36992.svg", nil, "").Code)

	var hist struct {
		Points []map[string]any `json:"points"`
		ID     *string          `json:"id"`
	}
	rec = do(h, "GET", "/api/v1/history/summary?metrics=tokens_count&from=2026-01-01", nil, "")
	require.Equal(t, 200, rec.Code)
	jsonBody(t, rec, &hist)
	require.Len(t, hist.Points, 1)
	assert.EqualValues(t, 647, hist.Points[0]["values"].(map[string]any)["tokens_count"])
	assert.Nil(t, hist.ID)
	rec = do(h, "GET", "/api/v1/history/summary?metrics=drop_table", nil, "")
	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "unknown_metric", errCode(t, rec)["code"])
	assert.Equal(t, 404, do(h, "GET", "/api/v1/history/tokens/1?metrics=price_usd", nil, "").Code)

	// Admin writes a run_requests row, readable back.
	rec = do(h, "POST", "/api/v1/admin/runs", map[string]string{"Authorization": "Bearer tok"}, `{"mode":"add","symbols":["NVDA"]}`)
	require.Equal(t, 202, rec.Code)
	var created map[string]any
	jsonBody(t, rec, &created)
	var mode string
	var symbols []string
	var requestedBy string
	require.NoError(t, pool.QueryRow(ctx, `SELECT mode, symbols, requested_by FROM run_requests WHERE id = $1`, int64(created["request_id"].(float64))).Scan(&mode, &symbols, &requestedBy))
	assert.Equal(t, "add", mode)
	assert.Equal(t, []string{"NVDA"}, symbols)
	assert.Equal(t, "admin-api", requestedBy)
	rec = do(h, "GET", "/api/v1/admin/runs/"+itoa(int64(created["request_id"].(float64))), map[string]string{"Authorization": "Bearer tok"}, "")
	require.Equal(t, 200, rec.Code)
	var rr map[string]any
	jsonBody(t, rec, &rr)
	assert.Nil(t, rr["outcome"])
	assert.Equal(t, []any{"NVDA"}, rr["symbols"])
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
