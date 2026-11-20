package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Store is what the API needs from the database layer; *store.Store (through pgStore) implements it and tests fake it.
type Store interface {
	Ping(ctx context.Context) error
	CurrentBuildMeta(ctx context.Context) (*store.BuildMeta, error)
	CurrentBuild(ctx context.Context) (*store.Build, error)
	BuildByID(ctx context.Context, id int64) (*store.Build, error)
	LastRun(ctx context.Context) (*store.Run, error)
	LastSuccessfulRunStart(ctx context.Context) (time.Time, error)
	ListRuns(ctx context.Context, limit int, cursor *store.RunCursor) ([]store.Run, error)
	GetRun(ctx context.Context, id int64) (*store.RunDetail, error)
	ListRunEvents(ctx context.Context, runID, afterID int64, level string, limit int) ([]store.RunEvent, *int64, error)
	RunStatus(ctx context.Context, id int64) (string, error)
	RunJournalClosed(ctx context.Context, id int64) (bool, error)
	BuildChangesForRuns(ctx context.Context, runIDs []int64) (map[int64]store.RunBuildChanges, error)
	QueryHistory(ctx context.Context, entity, id string, metrics []string, from, to *time.Time, step string, limit int) (*store.HistoryResult, error)
	GetLogo(ctx context.Context, kind, entityID string) (*store.Logo, error)
	CreateRunRequest(ctx context.Context, requestedBy, mode string, symbols []string) (int64, error)
	GetRunRequest(ctx context.Context, id int64) (*store.RunRequest, error)
}

// pgStore adds Ping to *store.Store.
type pgStore struct {
	*store.Store
	pool *pgxpool.Pool
}

func (p pgStore) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
