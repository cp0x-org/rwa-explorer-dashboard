package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CollectorLockName is the advisory lock every writer of collection data holds: a collection run, an import.
const CollectorLockName = "rwa-collector-run"

// ErrLocked is returned by TryLock when another session holds the lock.
var ErrLocked = errors.New("db: advisory lock is held by another session")

// Lock is a session-level advisory lock held on a dedicated pool connection.
type Lock struct {
	conn *pgxpool.Conn
	name string
	log  *slog.Logger
}

// TryLock takes the session advisory lock hashtext(name) without waiting. It returns ErrLocked when it is held elsewhere.
// Release must be called (also after ctx is cancelled).
func TryLock(ctx context.Context, pool *pgxpool.Pool, name string, log *slog.Logger) (*Lock, error) {
	if log == nil {
		log = slog.Default()
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: acquire connection: %w", err)
	}
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, name).Scan(&locked); err != nil {
		conn.Release()
		return nil, fmt.Errorf("db: advisory lock %s: %w", name, err)
	}
	if !locked {
		conn.Release()
		return nil, ErrLocked
	}
	return &Lock{conn: conn, name: name, log: log}, nil
}

// Conn is the connection that owns the lock (for work that must run in the locked session).
func (l *Lock) Conn() *pgxpool.Conn { return l.conn }

// Release unlocks and returns the connection to the pool. When the unlock fails the connection is closed, which
// releases the lock server-side: a locked connection never goes back to the pool.
func (l *Lock) Release(ctx context.Context) {
	uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var ok bool
	if err := l.conn.QueryRow(uctx, `SELECT pg_advisory_unlock(hashtext($1))`, l.name).Scan(&ok); err != nil || !ok {
		l.log.Warn("advisory unlock failed, closing connection", "lock", l.name, "err", err)
		_ = l.conn.Conn().Close(uctx)
	}
	l.conn.Release()
}
