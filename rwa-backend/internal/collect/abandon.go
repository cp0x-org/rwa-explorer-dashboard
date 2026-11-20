package collect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// AbandonRunningRuns marks runs left 'running' by a crashed process as failed with error 'abandoned'. It does so only
// while it holds the collector advisory lock: when another process holds it, a run may really be in progress and
// nothing is touched (0, nil). Call it once when the collector starts, after the migrations.
func AbandonRunningRuns(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (int64, error) {
	if log == nil {
		log = slog.Default()
	}
	lock, err := db.TryLock(ctx, pool, db.CollectorLockName, log)
	if errors.Is(err, db.ErrLocked) {
		log.Info("abandoned-run cleanup skipped: another collector run is in progress")
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("collect: abandon running runs: %w", err)
	}
	defer lock.Release(ctx)
	n, err := store.New(pool).AbandonRunningRuns(ctx)
	if err != nil {
		return 0, fmt.Errorf("collect: %w", err)
	}
	if n > 0 {
		log.Warn("runs left running by a previous process were marked failed (abandoned)", "count", n)
	}
	return n, nil
}
