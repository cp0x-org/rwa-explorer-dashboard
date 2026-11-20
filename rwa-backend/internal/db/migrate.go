package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationStatus is one migration and whether it is applied.
type MigrationStatus struct {
	Version   int64
	Source    string
	Applied   bool
	AppliedAt time.Time
}

// withProvider opens a short-lived database/sql handle (pgx stdlib) and a goose provider over the embedded files.
func withProvider(ctx context.Context, url string, fn func(*goose.Provider) error) error {
	cfg, err := ParseConfig(url)
	if err != nil {
		return err
	}
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	defer func() { _ = sqlDB.Close() }()
	return runProvider(ctx, sqlDB, fn)
}

func runProvider(ctx context.Context, sqlDB *sql.DB, fn func(*goose.Provider) error) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db: migrations fs: %w", err)
	}
	// A session advisory lock keeps concurrent `migrate` runs (several replicas starting) from racing.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("db: migration locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("db: goose provider: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("db: ping: %w", err)
	}
	return fn(p)
}

// Up applies all pending migrations.
func Up(ctx context.Context, url string) error {
	return withProvider(ctx, url, func(p *goose.Provider) error {
		if _, err := p.Up(ctx); err != nil {
			return fmt.Errorf("db: migrate up: %w", err)
		}
		return nil
	})
}

// DownTo rolls back until version is the current one (0 rolls everything back).
func DownTo(ctx context.Context, url string, version int64) error {
	return withProvider(ctx, url, func(p *goose.Provider) error {
		if _, err := p.DownTo(ctx, version); err != nil {
			return fmt.Errorf("db: migrate down to %d: %w", version, err)
		}
		return nil
	})
}

// Version returns the current applied version (0 when none).
func Version(ctx context.Context, url string) (int64, error) {
	var v int64
	err := withProvider(ctx, url, func(p *goose.Provider) error {
		var err error
		v, err = p.GetDBVersion(ctx)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("db: migrate version: %w", err)
	}
	return v, nil
}

// Status lists all known migrations with their state.
func Status(ctx context.Context, url string) ([]MigrationStatus, error) {
	var out []MigrationStatus
	err := withProvider(ctx, url, func(p *goose.Provider) error {
		st, err := p.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			out = append(out, MigrationStatus{
				Version:   s.Source.Version,
				Source:    s.Source.Path,
				Applied:   s.State == goose.StateApplied,
				AppliedAt: s.AppliedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("db: migrate status: %w", err)
	}
	return out, nil
}
