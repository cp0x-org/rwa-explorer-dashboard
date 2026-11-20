package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BuildRow is the input of InsertBuild.
type BuildRow struct {
	ID            int64 // optional: an id from ReserveBuildID; zero lets the database assign one
	RunID         int64
	GeneratedAt   time.Time
	SchemaVersion int
	ScoreVersion  string
	Settings      []byte // JSON, as in export.settings
	Summary       []byte // JSON
	Export        []byte // the exact JSON bytes the API serves (stored as BYTEA, key order preserved)
	ExportSHA256  string // hex sha256 of Export: the ETag
}

// BuildMeta is a builds row without the export document.
type BuildMeta struct {
	ID            int64
	RunID         int64
	GeneratedAt   time.Time
	SchemaVersion int
	ScoreVersion  string
	Settings      json.RawMessage
	Summary       json.RawMessage
	ExportSHA256  string
	IsCurrent     bool
}

// Build is a builds row with the export document.
type Build struct {
	BuildMeta
	Export []byte
}

// BuildChangesRow is a build_changes row.
type BuildChangesRow struct {
	BuildID            int64
	PreviousBuildID    *int64
	NewTokens          []int32
	RemovedTokens      []int32
	NewIssuers         []string
	NewPositionsCount  int
	GonePositionsCount int
	EligibleDelta      int
	Details            []byte // JSON object
}

const buildMetaColumns = `id, run_id, generated_at, schema_version, score_version, settings, summary, export_sha256, is_current`

func scanBuildMeta(row pgx.Row, extra ...any) (*BuildMeta, error) {
	var m BuildMeta
	dest := append([]any{&m.ID, &m.RunID, &m.GeneratedAt, &m.SchemaVersion, &m.ScoreVersion, &m.Settings, &m.Summary, &m.ExportSHA256, &m.IsCurrent}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &m, nil
}

// ReserveBuildID takes the next builds.id from the sequence, so the export can carry its own build_id before it is
// inserted. Sequences are not transactional: an id reserved by a rolled-back transaction is simply never used.
func (s *Store) ReserveBuildID(ctx context.Context) (int64, error) {
	var id int64
	if err := s.q.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('builds', 'id'))`).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: reserve build id: %w", err)
	}
	return id, nil
}

// InsertBuild inserts a non-current build (with b.ID when set, see ReserveBuildID). Low-level: production code publishes
// with PublishBuild. It takes a transaction so that InsertBuild, InsertBuildChanges and SetCurrentBuild commit together:
// if the transaction fails the previous build stays current.
func (s *Store) InsertBuild(ctx context.Context, tx pgx.Tx, b BuildRow) (int64, error) {
	var id int64
	var preset *int64
	if b.ID != 0 {
		preset = &b.ID
	}
	err := tx.QueryRow(ctx, `INSERT INTO builds (id, run_id, generated_at, schema_version, score_version, settings, summary, export, export_sha256)
		VALUES (COALESCE($1, nextval(pg_get_serial_sequence('builds', 'id'))), $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		preset, b.RunID, b.GeneratedAt, b.SchemaVersion, b.ScoreVersion, jsonOr(b.Settings, "{}"), jsonOr(b.Summary, "{}"), b.Export, b.ExportSHA256).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: insert build: %w", err)
	}
	return id, nil
}

// SetCurrentBuild (low-level; see PublishBuild) makes build id the only current one: it clears the flag on others, then sets it.
// A transaction-scoped advisory lock serializes concurrent switches.
func (s *Store) SetCurrentBuild(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('rwa-set-current-build'))`); err != nil {
		return fmt.Errorf("store: set current build: lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE builds SET is_current = FALSE WHERE is_current AND id <> $1`, id); err != nil {
		return fmt.Errorf("store: set current build: clear: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE builds SET is_current = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: set current build %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("store: set current build %d: %w", id, ErrNotFound)
	}
	return nil
}

// InsertBuildChanges (low-level; see PublishBuild) stores the diff against the previous build.
func (s *Store) InsertBuildChanges(ctx context.Context, tx pgx.Tx, c BuildChangesRow) error {
	if c.NewTokens == nil {
		c.NewTokens = []int32{}
	}
	if c.RemovedTokens == nil {
		c.RemovedTokens = []int32{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO build_changes (build_id, previous_build_id, new_tokens, removed_tokens, new_issuers,
			new_positions_count, gone_positions_count, eligible_delta, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.BuildID, c.PreviousBuildID, c.NewTokens, c.RemovedTokens, strs(c.NewIssuers),
		c.NewPositionsCount, c.GonePositionsCount, c.EligibleDelta, jsonOr(c.Details, "{}"))
	if err != nil {
		return fmt.Errorf("store: insert build changes %d: %w", c.BuildID, err)
	}
	return nil
}

// CurrentBuild returns the current build with its export, or ErrNotFound when there is none.
func (s *Store) CurrentBuild(ctx context.Context) (*Build, error) {
	return s.build(ctx, `WHERE is_current`)
}

// BuildByID returns a build with its export, or ErrNotFound.
func (s *Store) BuildByID(ctx context.Context, id int64) (*Build, error) {
	return s.build(ctx, `WHERE id = $1`, id)
}

func (s *Store) build(ctx context.Context, where string, args ...any) (*Build, error) {
	var export []byte
	m, err := scanBuildMeta(s.q.QueryRow(ctx, `SELECT `+buildMetaColumns+`, export FROM builds `+where+` LIMIT 1`, args...), &export)
	if err != nil {
		return nil, fmt.Errorf("store: get build: %w", notFound(err))
	}
	return &Build{BuildMeta: *m, Export: export}, nil
}

// CurrentBuildMeta returns the current build without its export (for /status), or ErrNotFound.
func (s *Store) CurrentBuildMeta(ctx context.Context) (*BuildMeta, error) {
	m, err := scanBuildMeta(s.q.QueryRow(ctx, `SELECT `+buildMetaColumns+` FROM builds WHERE is_current`))
	if err != nil {
		return nil, fmt.Errorf("store: current build meta: %w", notFound(err))
	}
	return m, nil
}

// BuildMetaByID returns a build without its export, or ErrNotFound.
func (s *Store) BuildMetaByID(ctx context.Context, id int64) (*BuildMeta, error) {
	m, err := scanBuildMeta(s.q.QueryRow(ctx, `SELECT `+buildMetaColumns+` FROM builds WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("store: build meta %d: %w", id, notFound(err))
	}
	return m, nil
}

// PublishBuild inserts a build, its changes (optional; BuildID is filled in) and makes it current, all on tx.
// The caller commits; if the transaction fails the previous build stays current.
func (s *Store) PublishBuild(ctx context.Context, tx pgx.Tx, b BuildRow, changes *BuildChangesRow) (int64, error) {
	id, err := s.InsertBuild(ctx, tx, b)
	if err != nil {
		return 0, err
	}
	if changes != nil {
		c := *changes
		c.BuildID = id
		if err := s.InsertBuildChanges(ctx, tx, c); err != nil {
			return 0, err
		}
	}
	if err := s.SetCurrentBuild(ctx, tx, id); err != nil {
		return 0, err
	}
	return id, nil
}
