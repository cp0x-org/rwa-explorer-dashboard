// Package testdb creates throwaway PostgreSQL databases for integration tests.
// Tests are skipped when TEST_DATABASE_URL is unset.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/db"
)

// EnvVar names the maintenance database URL used to create test databases.
const EnvVar = "TEST_DATABASE_URL"

// Empty creates a fresh empty database and returns its URL. The database is dropped at test cleanup.
func Empty(t testing.TB) string {
	t.Helper()
	admin := os.Getenv(EnvVar)
	if admin == "" {
		t.Skipf("%s is not set; skipping integration test", EnvVar)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("testdb: connect to maintenance database: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("testdb: random name: %v", err)
	}
	name := "rwa_test_" + hex.EncodeToString(b[:])
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("testdb: create database: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		c, err := pgx.Connect(cctx, admin)
		if err != nil {
			t.Logf("testdb: cleanup connect: %v", err)
			return
		}
		defer func() { _ = c.Close(cctx) }()
		if _, err := c.Exec(cctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("testdb: drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("testdb: parse %s: %v", EnvVar, err)
	}
	u.Path = "/" + name
	return u.String()
}

// Migrated creates a fresh database with all migrations applied and returns a pool for it.
func Migrated(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	u := Empty(t)
	ctx := context.Background()
	if err := db.Up(ctx, u); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}
	pool, err := db.Connect(ctx, u)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, u
}
