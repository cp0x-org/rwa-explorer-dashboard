package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/api"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
)

// dbWaitLimit is how long `rwa api` waits for an unreachable database at start before giving up.
const dbWaitLimit = 60 * time.Second

// runAPI implements `rwa api`: the HTTP API. It only checks that migrations are applied, never applies them.
func runAPI(args []string, _, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: rwa api")
		return 2
	}
	cfg, err := config.Load()
	if err == nil {
		err = cfg.ValidateAPI()
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rwa api:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := newLogger(cfg, stderr)

	pool, err := connectWithRetry(ctx, cfg.DatabaseURL, dbWaitLimit, log)
	if err != nil {
		log.Error("database unreachable", "err", err)
		return 1
	}
	defer closePool(pool, log)
	apiBanner(ctx, stderr, cfg)

	if err := ensureMigrated(ctx, cfg.DatabaseURL); err != nil {
		log.Error("database schema check failed", "err", err)
		return 1
	}
	if err := api.New(cfg, pool, log).Run(ctx); err != nil {
		log.Error("api stopped", "err", err)
		return 1
	}
	return 0
}

// connectWithRetry opens the pool, retrying with exponential backoff (1 s, doubling to 8 s) until limit has passed.
func connectWithRetry(ctx context.Context, dbURL string, limit time.Duration, log *slog.Logger) (*pgxpool.Pool, error) {
	deadline := time.Now().Add(limit)
	delay := time.Second
	for {
		pool, err := db.Connect(ctx, dbURL)
		if err == nil {
			return pool, nil
		}
		if ctx.Err() != nil || time.Now().Add(delay).After(deadline) {
			return nil, err
		}
		log.Warn("database not reachable yet, retrying", "err", err, "retry_in", delay.String())
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 8*time.Second)
	}
}
