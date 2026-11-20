// Package store is the PostgreSQL repository layer: runs, snapshots, current-state dimensions,
// append-only history, builds and logos. Methods work on a Querier, so the same code runs on the
// pool or inside a transaction (db.WithTx).
package store

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// Querier is implemented by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Store bundles a Querier. Create one per pool, or per transaction: store.New(tx).
type Store struct {
	q Querier
}

// New returns a Store over q (a *pgxpool.Pool or a pgx.Tx).
func New(q Querier) *Store { return &Store{q: q} }

// numeric converts a nullable decimal to a pgx NUMERIC value without precision loss.
func numeric(d model.Decimal) pgtype.Numeric {
	if d.V == nil {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: new(big.Int).Set(d.V.Coefficient()), Exp: d.V.Exponent(), Valid: true}
}

// strs makes a nil slice an empty one: a nil []string is NULL for pgx, but most array columns are NOT NULL.
func strs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// jsonOr returns b, or def when b is empty (for NOT NULL JSONB columns).
func jsonOr(b []byte, def string) []byte {
	if len(b) == 0 {
		return []byte(def)
	}
	return b
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// execBatch sends a batch and fails on the first error.
func (s *Store) execBatch(ctx context.Context, b *pgx.Batch, what string) error {
	if b.Len() == 0 {
		return nil
	}
	br := s.q.SendBatch(ctx, b)
	defer func() { _ = br.Close() }()
	for i := 0; i < b.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("store: %s (row %d): %w", what, i, err)
		}
	}
	return nil
}
