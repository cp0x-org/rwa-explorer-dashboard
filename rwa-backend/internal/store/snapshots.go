package store

import (
	"context"
	"fmt"
	"time"
)

// SnapshotRow is a snapshots row.
type SnapshotRow struct {
	ID            int64
	RunID         int64
	UnderlyingID  string
	FetchedAt     time.Time
	Status        string
	SchemaVersion int
	Payload       []byte // the 1.x snapshot document (JSON)
}

// InsertSnapshot stores one snapshot payload; payload_bytes is len(payload).
func (s *Store) InsertSnapshot(ctx context.Context, runID int64, underlyingID string, fetchedAt time.Time, status string, payload []byte) (int64, error) {
	var id int64
	err := s.q.QueryRow(ctx, `INSERT INTO raw_snapshots (run_id, underlying_id, fetched_at, status, payload, payload_bytes)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		runID, underlyingID, fetchedAt, status, payload, len(payload)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: insert snapshot %s: %w", underlyingID, err)
	}
	return id, nil
}

// LatestSnapshotsForEnabled returns the latest snapshot (by fetched_at, any status) of every enabled
// underlying that has one, ordered by fetched_at then underlying_id. compute must sort like 1.x
// (fetched_at, then assets.json order) itself: do not rely on this order for ties.
func (s *Store) LatestSnapshotsForEnabled(ctx context.Context) ([]SnapshotRow, error) {
	rows, err := s.q.Query(ctx, `SELECT id, run_id, underlying_id, fetched_at, status, schema_version, payload FROM (
			SELECT DISTINCT ON (sn.underlying_id) sn.id, sn.run_id, sn.underlying_id, sn.fetched_at, sn.status,
				sn.schema_version, sn.payload
			FROM raw_snapshots sn JOIN catalog_underlyings u ON u.id = sn.underlying_id
			WHERE u.enabled ORDER BY sn.underlying_id, sn.fetched_at DESC, sn.id DESC
		) latest ORDER BY fetched_at, underlying_id`)
	if err != nil {
		return nil, fmt.Errorf("store: latest snapshots: %w", err)
	}
	defer rows.Close()
	var out []SnapshotRow
	for rows.Next() {
		var r SnapshotRow
		if err := rows.Scan(&r.ID, &r.RunID, &r.UnderlyingID, &r.FetchedAt, &r.Status, &r.SchemaVersion, &r.Payload); err != nil {
			return nil, fmt.Errorf("store: latest snapshots: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: latest snapshots: %w", err)
	}
	return out, nil
}

// DeleteSnapshotsOlderThan deletes snapshots fetched before cutoff. It never deletes the latest snapshot of
// an underlying (whatever its status), which compute reads. It also keeps the latest status 'ok' snapshot of
// each underlying, so the last good data survives a streak of non-ok snapshots.
func (s *Store) DeleteSnapshotsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `DELETE FROM raw_snapshots WHERE fetched_at < $1 AND id NOT IN (
			SELECT DISTINCT ON (underlying_id) id FROM raw_snapshots ORDER BY underlying_id, fetched_at DESC, id DESC
		) AND id NOT IN (
			SELECT DISTINCT ON (underlying_id) id FROM raw_snapshots WHERE status = 'ok' ORDER BY underlying_id, fetched_at DESC, id DESC
		)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: delete old snapshots: %w", err)
	}
	return tag.RowsAffected(), nil
}
