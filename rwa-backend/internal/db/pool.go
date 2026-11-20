// Package db provides the PostgreSQL connection pool, a transaction helper and embedded migrations.
package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ParseConfig parses a database URL with pgxpool (so pool_* parameters are understood and not sent to the
// server) and applies defaults: 10s connect timeout, 8 max and 1 min connections unless the URL sets them,
// and UTC for every scanned timestamptz.
func ParseConfig(dbURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse database url: %w", err)
	}
	q := map[string]bool{}
	if u, err := url.Parse(dbURL); err == nil {
		for k := range u.Query() {
			q[k] = true
		}
	}
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	}
	if !q["pool_max_conns"] {
		cfg.MaxConns = 8
	}
	if !q["pool_min_conns"] {
		cfg.MinConns = 1
	}
	cfg.AfterConnect = func(_ context.Context, c *pgx.Conn) error {
		c.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}
	return cfg, nil
}

// Connect opens a pool for url and verifies it with a ping.
func Connect(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	cfg, err := ParseConfig(dbURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// WithTx runs fn in a transaction: it commits when fn returns nil and rolls back on an error or a panic
// (the panic is re-raised).
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			// Rollback must not depend on a cancelled ctx.
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("db: rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}
